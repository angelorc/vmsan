import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { execSync } from "node:child_process";
import { consola } from "consola";
import { createVmsan } from "../../context.ts";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { handleCommandError, vmNotFoundError } from "../../errors/index.ts";
import { table, toError } from "../../lib/utils.ts";

interface ConntrackEntry {
  proto: string;
  src: string;
  dst: string;
  dport: string;
  state: string;
}

function parseConntrackOutput(output: string, meshIp: string): ConntrackEntry[] {
  const entries: ConntrackEntry[] = [];

  for (const line of output.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || !trimmed.includes(meshIp)) continue;

    // conntrack -L output format:
    // tcp  6 431999 ESTABLISHED src=10.90.0.1 dst=10.90.0.2 sport=38456 dport=5432 ...
    const protoMatch = /^(\w+)/.exec(trimmed);
    const srcMatch = /src=(\S+)/.exec(trimmed);
    const dstMatch = /dst=(\S+)/.exec(trimmed);
    const dportMatch = /dport=(\d+)/.exec(trimmed);
    const stateMatch =
      /\b(ESTABLISHED|SYN_SENT|SYN_RECV|FIN_WAIT|CLOSE_WAIT|LAST_ACK|TIME_WAIT|CLOSE|LISTEN|NONE)\b/.exec(
        trimmed,
      );

    if (!protoMatch || !srcMatch || !dstMatch || !dportMatch) continue;

    entries.push({
      proto: protoMatch[1],
      src: srcMatch[1],
      dst: dstMatch[1],
      dport: dportMatch[1],
      state: stateMatch?.[1] ?? "-",
    });
  }

  return entries;
}

const connectionsCommand = defineCommand({
  meta: {
    name: "connections",
    description: "Show active mesh connections for a VM",
  },
  args: {
    vmId: {
      type: "positional",
      description: "VM to inspect",
      required: true,
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("network connections");

    try {
      const vmsan = await createVmsan();
      const state = vmsan.get(args.vmId);
      if (!state) {
        throw vmNotFoundError(args.vmId);
      }

      // Use meshIp if available, fall back to guestIp
      const filterIp = state.network.meshIp || state.network.guestIp;

      let conntrackOutput = "";
      try {
        conntrackOutput = execSync("conntrack -L 2>/dev/null", {
          encoding: "utf-8",
          timeout: 5000,
        });
      } catch (err) {
        consola.debug(`conntrack -L failed: ${toError(err).message}`);
        conntrackOutput = "";
      }

      const entries = parseConntrackOutput(conntrackOutput, filterIp);

      if (getOutputMode() === "json") {
        cmdLog.set({
          vmId: args.vmId,
          filterIp,
          count: entries.length,
          connections: entries,
        });
      } else {
        if (entries.length === 0) {
          consola.info(`No active connections for ${args.vmId} (${filterIp})`);
        } else {
          const output = table<ConntrackEntry>({
            rows: entries,
            columns: {
              PROTO: { value: (e) => e.proto },
              SRC: { value: (e) => e.src },
              DST: { value: (e) => e.dst },
              DPORT: { value: (e) => e.dport },
              STATE: { value: (e) => e.state },
            },
          });
          consola.log(output);
        }

        cmdLog.set({
          vmId: args.vmId,
          filterIp,
          count: entries.length,
        });
      }

      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default connectionsCommand as CommandDef;
