import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { handleCommandError } from "../../errors/index.ts";
import { ServerClient } from "../../lib/server-client.ts";

const checkCommand = defineCommand({
  meta: {
    name: "check",
    description: "Check host connectivity and status",
  },
  args: {
    name: {
      type: "positional",
      description: "Name of the host to check",
      required: true,
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("hosts check");
    const log = consola.withTag("hosts");

    try {
      const client = ServerClient.fromEnv();

      log.start(`Checking host "${args.name}"...`);
      const host = await client.findHostByName(args.name);

      if (!host) {
        consola.error(`Host "${args.name}" not found.`);
        cmdLog.set({ name: args.name, found: false });
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      if (getOutputMode() === "json") {
        cmdLog.set({ host });
      } else {
        log.log(`  Name:      ${host.name}`);
        log.log(`  Address:   ${host.address}`);
        log.log(`  Status:    ${host.status}`);
        log.log(`  VMs:       ${host.vm_count}`);
        if (host.resources) {
          log.log(`  CPU:       ${host.resources.cpus} cores`);
          log.log(`  Memory:    ${Math.round(host.resources.memory_mb / 1024)} GB`);
          log.log(`  Disk:      ${host.resources.disk_gb} GB`);
        }
        if (host.last_heartbeat) {
          log.log(`  Heartbeat: ${host.last_heartbeat}`);
        }

        if (host.status === "active") {
          log.success(`Host "${args.name}" is reachable and active.`);
        } else {
          consola.warn(`Host "${args.name}" status: ${host.status}`);
        }
      }

      cmdLog.set({ name: args.name, id: host.id, status: host.status });
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default checkCommand as CommandDef;
