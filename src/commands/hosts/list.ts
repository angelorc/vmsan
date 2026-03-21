import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { handleCommandError } from "../../errors/index.ts";
import { table } from "../../lib/utils.ts";
import { ServerClient, type HostInfo } from "../../lib/server-client.ts";

function formatMemory(mb: number): string {
  if (mb >= 1024) return `${Math.round(mb / 1024)} GB`;
  return `${mb} MB`;
}

const STATUS_COLORS: Record<string, string> = {
  active: "\x1b[32m", // green
  degraded: "\x1b[33m", // yellow
  offline: "\x1b[2;90m", // dim gray
  draining: "\x1b[35m", // magenta
};
const RESET = "\x1b[0m";

function colorStatus(status: string): string {
  const color = STATUS_COLORS[status] || "";
  return `${color}${status}${RESET}`;
}

const listCommand = defineCommand({
  meta: {
    name: "list",
    description: "List registered hosts",
  },
  args: {},
  async run() {
    const cmdLog = createCommandLogger("hosts list");
    const log = consola.withTag("hosts");

    try {
      const client = ServerClient.fromEnv();
      const hosts = await client.listHosts();

      if (hosts.length === 0) {
        if (getOutputMode() === "json") {
          cmdLog.set({ count: 0, hosts: [] });
        } else {
          log.log("No hosts registered.");
          cmdLog.set({ count: 0 });
        }
        cmdLog.emit();
        return;
      }

      if (getOutputMode() === "json") {
        cmdLog.set({ count: hosts.length, hosts });
      } else {
        const output = table<HostInfo>({
          rows: hosts,
          columns: {
            NAME: { value: (h) => h.name },
            ADDRESS: { value: (h) => h.address },
            STATUS: {
              value: (h) => h.status,
              color: (h) => colorStatus(h.status),
            },
            VMs: { value: (h) => h.vm_count },
            CPU: { value: (h) => (h.resources ? String(h.resources.cpus) : "-") },
            MEMORY: { value: (h) => (h.resources ? formatMemory(h.resources.memory_mb) : "-") },
          },
        });

        log.log(output);
        cmdLog.set({ count: hosts.length });
      }
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default listCommand as CommandDef;
