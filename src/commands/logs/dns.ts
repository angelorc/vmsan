import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { createReadStream, existsSync, watchFile, unwatchFile } from "node:fs";
import { createInterface } from "node:readline";
import { consola } from "consola";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { handleCommandError } from "../../errors/index.ts";

interface DNSLogEntry {
  event: string;
  domain: string;
  result: string;
  policy: string;
  latency_ms: number;
  vmId: string;
  ts: string;
}

const GREEN = "\x1b[32m";
const RED = "\x1b[31m";
const DIM = "\x1b[2;90m";
const RESET = "\x1b[0m";

function formatTimestamp(ts: string): string {
  try {
    const d = new Date(ts);
    return d.toLocaleTimeString("en-US", { hour12: false });
  } catch {
    return ts;
  }
}

function formatHumanLine(entry: DNSLogEntry): string {
  const time = formatTimestamp(entry.ts);
  const isAllowed = entry.policy === "allow";
  const color = isAllowed ? GREEN : RED;
  const label = isAllowed ? "ALLOW" : "DENY ";
  const result = entry.result || "-";
  const latency = entry.latency_ms != null ? `(${entry.latency_ms}ms)` : "";

  return `${DIM}[${time}]${RESET} ${color}${label}${RESET}  ${entry.domain} → ${result} ${latency}`;
}

function shouldInclude(
  entry: DNSLogEntry,
  allowedOnly: boolean,
  deniedOnly: boolean,
): boolean {
  if (allowedOnly && entry.policy !== "allow") return false;
  if (deniedOnly && entry.policy !== "deny") return false;
  return true;
}

function dnsLogPath(vmId: string): string {
  return `/tmp/vmsan-dns-${vmId}.log`;
}

async function readExistingLines(
  filePath: string,
  opts: { allowedOnly: boolean; deniedOnly: boolean; json: boolean },
): Promise<number> {
  return new Promise((resolve, reject) => {
    let bytesRead = 0;
    const stream = createReadStream(filePath, { encoding: "utf-8" });
    const rl = createInterface({ input: stream, crlfDelay: Infinity });

    rl.on("line", (line) => {
      bytesRead += Buffer.byteLength(line, "utf-8") + 1; // +1 for newline
      if (!line.trim()) return;
      try {
        const entry: DNSLogEntry = JSON.parse(line);
        if (!shouldInclude(entry, opts.allowedOnly, opts.deniedOnly)) return;
        if (opts.json) {
          consola.log(line);
        } else {
          consola.log(formatHumanLine(entry));
        }
      } catch {
        // Skip malformed lines
      }
    });

    rl.on("close", () => resolve(bytesRead));
    rl.on("error", reject);
  });
}

async function tailFile(
  filePath: string,
  startOffset: number,
  opts: { allowedOnly: boolean; deniedOnly: boolean; json: boolean },
): Promise<void> {
  let offset = startOffset;

  return new Promise((_resolve, _reject) => {
    const onChange = () => {
      const stream = createReadStream(filePath, {
        encoding: "utf-8",
        start: offset,
      });
      const rl = createInterface({ input: stream, crlfDelay: Infinity });

      rl.on("line", (line) => {
        offset += Buffer.byteLength(line, "utf-8") + 1;
        if (!line.trim()) return;
        try {
          const entry: DNSLogEntry = JSON.parse(line);
          if (!shouldInclude(entry, opts.allowedOnly, opts.deniedOnly)) return;
          if (opts.json) {
            consola.log(line);
          } else {
            consola.log(formatHumanLine(entry));
          }
        } catch {
          // Skip malformed lines
        }
      });
    };

    watchFile(filePath, { interval: 500 }, onChange);

    // Clean up on SIGINT
    const cleanup = () => {
      unwatchFile(filePath, onChange);
      process.exit(0);
    };
    process.on("SIGINT", cleanup);
    process.on("SIGTERM", cleanup);
  });
}

const dnsLogsCommand = defineCommand({
  meta: {
    name: "dns",
    description: "View DNS query logs for a VM",
  },
  args: {
    vmId: {
      type: "positional",
      description: "VM identifier",
      required: true,
    },
    follow: {
      type: "boolean",
      alias: "f",
      default: false,
      description: "Follow log output (tail -f behavior)",
    },
    allowedOnly: {
      type: "boolean",
      default: false,
      description: "Show only allowed DNS queries",
    },
    deniedOnly: {
      type: "boolean",
      default: false,
      description: "Show only denied DNS queries",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("logs:dns");
    const isJson = getOutputMode() === "json";

    try {
      const filePath = dnsLogPath(args.vmId);

      if (!existsSync(filePath)) {
        if (isJson) {
          cmdLog.set({ vmId: args.vmId, error: "no_dns_log" });
          cmdLog.emit();
        } else {
          consola.warn(`No DNS log found for VM ${args.vmId}`);
          consola.info(`Expected log at: ${filePath}`);
        }
        process.exitCode = 1;
        return;
      }

      const filterOpts = {
        allowedOnly: args.allowedOnly,
        deniedOnly: args.deniedOnly,
        json: isJson,
      };

      const bytesRead = await readExistingLines(filePath, filterOpts);

      if (args.follow) {
        await tailFile(filePath, bytesRead, filterOpts);
      }

      cmdLog.set({ vmId: args.vmId, follow: args.follow });
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default dnsLogsCommand as CommandDef;
