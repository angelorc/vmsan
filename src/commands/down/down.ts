import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createInterface } from "node:readline";
import { createVmsan } from "../../context.ts";
import { normalizeToml } from "../../lib/toml/parser.ts";
import { loadProjectConfig } from "../../lib/project.ts";
import { buildDependencyGraph } from "../../lib/deploy/graph.ts";
import { removeDeployHash } from "../../lib/deploy/hash.ts";
import { handleCommandError } from "../../errors/index.ts";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { toError } from "../../lib/utils.ts";

async function confirm(message: string): Promise<boolean> {
  const rl = createInterface({ input: process.stdin, output: process.stdout });
  return new Promise((resolve) => {
    rl.question(`${message} [y/N] `, (answer) => {
      rl.close();
      resolve(answer.toLowerCase() === "y");
    });
  });
}

const downCommand = defineCommand({
  meta: {
    name: "down",
    description: "Stop all services for the current project",
  },
  args: {
    config: {
      type: "string",
      description: "Path to vmsan.toml (default: ./vmsan.toml)",
    },
    destroy: {
      type: "boolean",
      default: false,
      description: "Remove all VMs and data after stopping",
    },
    force: {
      type: "boolean",
      alias: "f",
      default: false,
      description: "Skip confirmation prompt for --destroy",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("down");

    try {
      // 1. Load project config
      const { config, project } = loadProjectConfig(args.config);

      // 2. Create VMService
      const vmService = await createVmsan();

      // 5. Find VMs for this project
      const projectVms = vmService.list().filter((vm) => vm.project === project);

      if (projectVms.length === 0) {
        consola.info(`No VMs found for project "${project}"`);
        return;
      }

      // 6. Confirm destruction if --destroy
      if (args.destroy && !args.force) {
        consola.warn(
          `This will stop and permanently delete ${projectVms.length} VM(s) for project "${project}"`,
        );
        const confirmed = await confirm("This will delete all data. Continue?");
        if (!confirmed) {
          consola.info("Aborted");
          return;
        }
      }

      // 7. Build dependency graph for reverse order shutdown
      const services = normalizeToml(config);
      const accessories = config.accessories ?? {};
      const depInput: Record<string, { depends_on?: string[] }> = {};
      for (const [name, svc] of Object.entries(services)) {
        depInput[name] = { depends_on: svc.depends_on };
      }
      const graph = buildDependencyGraph(depInput, accessories);

      // Build reverse order: stop dependents first, then dependencies
      const reverseOrder = graph.reverseOrder;

      // Map service names to VMs, and include any VMs not in the graph
      const vmByService = new Map<string, (typeof projectVms)[number]>();
      const unmappedVms: typeof projectVms = [];

      for (const vm of projectVms) {
        if (vm.network.service) {
          vmByService.set(vm.network.service, vm);
        } else {
          unmappedVms.push(vm);
        }
      }

      // Ordered list: known services in reverse dep order, then unmapped VMs
      const orderedVmIds: string[] = [];
      for (const serviceName of reverseOrder) {
        const vm = vmByService.get(serviceName);
        if (vm) {
          orderedVmIds.push(vm.id);
          vmByService.delete(serviceName);
        }
      }
      // Add any remaining mapped VMs not in the graph
      for (const vm of vmByService.values()) {
        orderedVmIds.push(vm.id);
      }
      // Add unmapped VMs
      for (const vm of unmappedVms) {
        orderedVmIds.push(vm.id);
      }

      consola.info(`Stopping ${orderedVmIds.length} VM(s) for project "${project}"`);

      // 8. Stop VMs in order
      const results: { vmId: string; service: string; action: string; success: boolean }[] = [];
      let hasErrors = false;

      for (const vmId of orderedVmIds) {
        const vm = vmService.get(vmId);
        if (!vm) continue;

        const serviceName = vm.network.service ?? vmId;

        if (vm.status === "stopped") {
          consola.info(`[${serviceName}] Already stopped`);
          if (args.destroy) {
            consola.start(`[${serviceName}] Removing VM ${vmId}...`);
            const removeResult = await vmService.remove(vmId, { force: true });
            if (removeResult.success) {
              removeDeployHash(vmId);
              consola.success(`[${serviceName}] Removed`);
              results.push({ vmId, service: serviceName, action: "removed", success: true });
            } else {
              consola.error(
                `[${serviceName}] Failed to remove: ${removeResult.error?.message ?? "unknown error"}`,
              );
              results.push({ vmId, service: serviceName, action: "remove_failed", success: false });
              hasErrors = true;
            }
          } else {
            results.push({ vmId, service: serviceName, action: "already_stopped", success: true });
          }
          continue;
        }

        // Stop the VM
        consola.start(`[${serviceName}] Stopping VM ${vmId}...`);
        const stopResult = await vmService.stop(vmId);

        if (stopResult.success || stopResult.alreadyStopped) {
          consola.success(`[${serviceName}] Stopped`);

          if (args.destroy) {
            consola.start(`[${serviceName}] Removing VM ${vmId}...`);
            const removeResult = await vmService.remove(vmId, { force: true });
            if (removeResult.success) {
              removeDeployHash(vmId);
              consola.success(`[${serviceName}] Removed`);
              results.push({ vmId, service: serviceName, action: "removed", success: true });
            } else {
              consola.error(
                `[${serviceName}] Failed to remove: ${removeResult.error?.message ?? "unknown error"}`,
              );
              results.push({ vmId, service: serviceName, action: "remove_failed", success: false });
              hasErrors = true;
            }
          } else {
            results.push({ vmId, service: serviceName, action: "stopped", success: true });
          }
        } else {
          const errorMsg = stopResult.error?.message ?? "unknown error";
          consola.error(`[${serviceName}] Failed to stop: ${errorMsg}`);
          results.push({ vmId, service: serviceName, action: "stop_failed", success: false });
          hasErrors = true;
        }
      }

      // 9. Report
      if (getOutputMode() === "json") {
        cmdLog.set({
          project,
          destroy: args.destroy,
          results,
          success: !hasErrors,
        });
        cmdLog.emit();
      } else {
        consola.log("");
        if (!hasErrors) {
          if (args.destroy) {
            consola.success(`All services for "${project}" have been removed`);
          } else {
            consola.success(`All services for "${project}" have been stopped`);
          }
        } else {
          consola.error("Some operations failed. Check the output above for details.");
          process.exitCode = 1;
        }
      }
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default downCommand as CommandDef;
