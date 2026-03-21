import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { resolve, basename } from "node:path";
import { existsSync } from "node:fs";
import { createVmsan } from "../../context.ts";
import { loadVmsanToml } from "../../lib/toml/parser.ts";
import { AgentClient } from "../../services/agent.ts";
import { handleCommandError } from "../../errors/index.ts";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { table, toError } from "../../lib/utils.ts";
import type { VmState } from "../../lib/vm-state.ts";

interface ServiceStatus {
  service: string;
  vmId: string;
  status: string;
  health: string;
  memory: string;
  endpoint: string;
}

async function checkHealth(vm: VmState): Promise<string> {
  if (vm.status !== "running" || !vm.agentToken) {
    return "-";
  }

  try {
    const agent = new AgentClient(`http://${vm.network.guestIp}:${vm.agentPort}`, vm.agentToken);
    const result = await agent.health();
    return result.status === "ok" || result.status === "healthy" ? "healthy" : result.status;
  } catch (err) {
    consola.debug(`Health check failed for ${vm.id}: ${toError(err).message}`);
    return "unreachable";
  }
}

const statusCommand = defineCommand({
  meta: {
    name: "status",
    description: "Show project service status overview",
  },
  args: {
    config: {
      type: "string",
      description: "Path to vmsan.toml (default: ./vmsan.toml)",
    },
    json: {
      type: "boolean",
      default: false,
      description: "Output as JSON",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("status");

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
      const config = loadVmsanToml(configPath);

      // 3. Determine project name
      const sourceDir = resolve(configPath, "..");
      const project = basename(sourceDir);

      // 4. Create VMService
      const vmService = await createVmsan();

      // 5. Find VMs for this project
      const projectVms = vmService.list().filter((vm) => vm.project === project);

      if (projectVms.length === 0) {
        if (getOutputMode() === "json" || args.json) {
          cmdLog.set({ project, services: [] });
          cmdLog.emit();
        } else {
          consola.info(`No VMs found for project "${project}". Run "vmsan up" to deploy.`);
        }
        return;
      }

      // 6. Check health for each VM (with 2s timeout per check)
      const statuses: ServiceStatus[] = [];

      for (const vm of projectVms) {
        const serviceName = vm.network.service ?? vm.id;
        const health = await checkHealth(vm);
        const endpoint = vm.network.service
          ? `${vm.network.service}.${project}.vmsan.internal`
          : "-";

        statuses.push({
          service: serviceName,
          vmId: vm.id,
          status: vm.status,
          health,
          memory: `${vm.memSizeMib} MB`,
          endpoint,
        });
      }

      // 7. Display results
      if (getOutputMode() === "json" || args.json) {
        cmdLog.set({ project, services: statuses });
        cmdLog.emit();
      } else {
        const statusColor = (status: string): string => {
          if (status === "running") return `\x1b[32m${status}\x1b[0m`;
          if (status === "stopped") return `\x1b[2;90m${status}\x1b[0m`;
          if (status === "error") return `\x1b[31m${status}\x1b[0m`;
          return status;
        };

        const healthColor = (health: string): string => {
          if (health === "healthy") return `\x1b[32m${health}\x1b[0m`;
          if (health === "unreachable") return `\x1b[31m${health}\x1b[0m`;
          return health;
        };

        consola.log("");
        consola.log(
          table({
            rows: statuses,
            columns: {
              SERVICE: { value: (r) => r.service },
              "VM ID": { value: (r) => r.vmId },
              STATUS: {
                value: (r) => r.status,
                color: (r) => statusColor(r.status),
              },
              HEALTH: {
                value: (r) => r.health,
                color: (r) => healthColor(r.health),
              },
              MEMORY: { value: (r) => r.memory },
              ENDPOINT: { value: (r) => r.endpoint },
            },
          }),
        );
        consola.log("");
      }
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default statusCommand as CommandDef;
