import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { resolve, basename } from "node:path";
import { existsSync } from "node:fs";
import { createVmsan } from "../../context.ts";
import { loadVmsanToml, normalizeToml } from "../../lib/toml/parser.ts";
import type { ServiceConfig } from "../../lib/toml/parser.ts";
import { buildDependencyGraph } from "../../lib/deploy/graph.ts";
import { uploadSource } from "../../lib/deploy/upload.ts";
import { executeBuild, startApp } from "../../lib/deploy/build.ts";
import { executeRelease } from "../../lib/deploy/release.ts";
import { AgentClient } from "../../services/agent.ts";
import { handleCommandError } from "../../errors/index.ts";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { table, toError } from "../../lib/utils.ts";
import type { VmState } from "../../lib/vm-state.ts";

interface RedeployResult {
  service: string;
  vmId: string;
  status: "deployed" | "failed" | "skipped";
  error?: string;
  durationMs: number;
}

function createAgentClient(state: VmState): AgentClient {
  const agentUrl = `http://${state.network.guestIp}:${state.agentPort}`;
  return new AgentClient(agentUrl, state.agentToken!);
}

async function stopRunningApp(agent: AgentClient): Promise<void> {
  try {
    // Kill any running app processes via the agent
    await agent.runCommand("sh", ["-c", "pkill -f '/app' || true"]);
  } catch {
    // Best-effort: app may not be running
  }
}

async function redeployService(
  name: string,
  vm: VmState,
  config: ServiceConfig,
  deployConfig: { release?: string } | undefined,
  sourceDir: string,
  env: Record<string, string>,
): Promise<RedeployResult> {
  const startTime = Date.now();

  try {
    if (!vm.agentToken) {
      throw new Error(`VM ${vm.id} has no agent token`);
    }

    const agent = createAgentClient(vm);

    // 1. Stop running app
    consola.start(`[${name}] Stopping running app...`);
    await stopRunningApp(agent);

    // 2. Upload new source
    consola.start(`[${name}] Uploading source...`);
    await uploadSource({ sourceDir, agent, targetDir: "/app" });

    // 3. Build
    if (config.build) {
      consola.start(`[${name}] Running build...`);
      const buildResult = await executeBuild({
        buildCommand: config.build,
        agent,
        cwd: "/app",
        env: { ...config.env, ...env },
      });
      if (!buildResult.success) {
        throw new Error(`Build failed with exit code ${buildResult.exitCode}`);
      }
    }

    // 4. Release
    if (deployConfig?.release) {
      consola.start(`[${name}] Running release command...`);
      const releaseResult = await executeRelease({
        command: deployConfig.release,
        agent,
      });
      if (!releaseResult.success) {
        throw new Error(`Release command failed with exit code ${releaseResult.exitCode}`);
      }
    }

    // 5. Start app
    if (config.start) {
      consola.start(`[${name}] Starting app...`);
      await startApp({
        startCommand: config.start,
        agent,
        cwd: "/app",
        env: { ...config.env, ...env },
      });
    }

    consola.success(`[${name}] Re-deployed successfully`);
    return {
      service: name,
      vmId: vm.id,
      status: "deployed",
      durationMs: Date.now() - startTime,
    };
  } catch (err) {
    const errorMessage = toError(err).message;
    consola.error(`[${name}] Re-deploy failed: ${errorMessage}`);
    return {
      service: name,
      vmId: vm.id,
      status: "failed",
      error: errorMessage,
      durationMs: Date.now() - startTime,
    };
  }
}

const deployCommand = defineCommand({
  meta: {
    name: "deploy",
    description: "Re-deploy services without recreating VMs",
  },
  args: {
    service: {
      type: "positional",
      description: "Service name to deploy (deploys all if omitted)",
      required: false,
    },
    config: {
      type: "string",
      description: "Path to vmsan.toml (default: ./vmsan.toml)",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("deploy");

    try {
      // 1. Find vmsan.toml
      const configPath = resolve(args.config || "vmsan.toml");
      if (!existsSync(configPath)) {
        consola.error(`Configuration file not found: ${configPath}`);
        consola.info('Run "vmsan init" to create a vmsan.toml');
        process.exitCode = 1;
        return;
      }

      // 2. Load and parse
      consola.start(`Loading ${configPath}`);
      const config = loadVmsanToml(configPath);

      // 3. Determine project name
      const sourceDir = resolve(configPath, "..");
      const project = basename(sourceDir);

      // 4. Create VMService
      const vmService = await createVmsan();

      // 5. Normalize services and determine what to deploy
      const services = normalizeToml(config);
      const accessories = config.accessories ?? {};
      const targetService = args.service as string | undefined;

      // 6. Find existing VMs for this project
      const projectVms = vmService.list().filter((vm) => vm.project === project);

      if (projectVms.length === 0) {
        consola.error(`No running VMs found for project "${project}". Run "vmsan up" first.`);
        process.exitCode = 1;
        return;
      }

      // 7. Determine services to re-deploy
      let servicesToDeploy: string[];

      if (targetService) {
        if (!services[targetService]) {
          consola.error(`Service "${targetService}" not found in vmsan.toml`);
          process.exitCode = 1;
          return;
        }
        servicesToDeploy = [targetService];
      } else {
        // Deploy all non-accessory services
        servicesToDeploy = Object.keys(services).filter((name) => !(name in accessories));
      }

      if (servicesToDeploy.length === 0) {
        consola.warn("No services to deploy");
        return;
      }

      // 8. Build dependency graph for ordering
      const depInput: Record<string, { depends_on?: string[] }> = {};
      for (const [name, svc] of Object.entries(services)) {
        depInput[name] = { depends_on: svc.depends_on };
      }
      const graph = buildDependencyGraph(depInput, accessories);

      // Filter to only services we're deploying, in dependency order
      const orderedServices = graph.order.filter((name) => servicesToDeploy.includes(name));

      consola.info(`Re-deploying ${orderedServices.length} service(s) for project "${project}"`);

      // 9. Re-deploy each service
      const results: RedeployResult[] = [];

      for (const serviceName of orderedServices) {
        const serviceConfig = services[serviceName];
        const vm = projectVms.find((v) => v.network.service === serviceName);

        if (!vm) {
          consola.warn(`[${serviceName}] No VM found, skipping (run "vmsan up" to create)`);
          results.push({
            service: serviceName,
            vmId: "-",
            status: "skipped",
            durationMs: 0,
          });
          continue;
        }

        if (vm.status !== "running") {
          consola.warn(`[${serviceName}] VM ${vm.id} is ${vm.status}, skipping`);
          results.push({
            service: serviceName,
            vmId: vm.id,
            status: "skipped",
            durationMs: 0,
          });
          continue;
        }

        const result = await redeployService(
          serviceName,
          vm,
          serviceConfig,
          config.deploy,
          sourceDir,
          {},
        );
        results.push(result);

        if (result.status === "failed") {
          consola.error(`Stopping deploy due to failure in "${serviceName}"`);
          break;
        }
      }

      // 10. Display results
      if (getOutputMode() === "json") {
        cmdLog.set({
          project,
          results,
          success: results.every((r) => r.status !== "failed"),
        });
        cmdLog.emit();
      } else {
        consola.log("");
        if (results.length > 0) {
          const statusColor = (status: string): string => {
            if (status === "deployed") return `\x1b[32m${status}\x1b[0m`;
            if (status === "skipped") return `\x1b[33m${status}\x1b[0m`;
            if (status === "failed") return `\x1b[31m${status}\x1b[0m`;
            return status;
          };

          consola.log(
            table({
              rows: results,
              columns: {
                SERVICE: { value: (r) => r.service },
                STATUS: {
                  value: (r) => r.status,
                  color: (r) => statusColor(r.status),
                },
                VM: { value: (r) => r.vmId },
                DURATION: { value: (r) => `${Math.round(r.durationMs / 1000)}s` },
              },
            }),
          );
          consola.log("");
        }

        const allSucceeded = results.every((r) => r.status !== "failed");
        if (allSucceeded) {
          consola.success("Deploy complete");
        } else {
          consola.error("Deploy failed");
          process.exitCode = 1;
        }
      }
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default deployCommand as CommandDef;
