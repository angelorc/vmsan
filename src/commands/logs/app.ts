import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createVmsan } from "../../context.ts";
import { loadProjectConfig } from "../../lib/project.ts";
import { createAgentClient } from "../../lib/deploy/agent-client.ts";
import { handleCommandError } from "../../errors/index.ts";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { toError } from "../../lib/utils.ts";
import type { VmState } from "../../lib/vm-state.ts";

const SERVICE_COLORS = [
  "\x1b[34m", // blue
  "\x1b[32m", // green
  "\x1b[33m", // yellow
  "\x1b[35m", // magenta
  "\x1b[36m", // cyan
  "\x1b[91m", // bright red
] as const;

const DIM = "\x1b[2;90m";
const RESET = "\x1b[0m";

function colorForIndex(index: number): string {
  return SERVICE_COLORS[index % SERVICE_COLORS.length];
}

function buildJournalctlCommand(lines: number, follow: boolean, timestamps: boolean): string {
  const parts = ["journalctl", "--no-hostname"];
  if (timestamps) {
    parts.push("-o", "short");
  } else {
    parts.push("-o", "cat");
  }
  parts.push("-n", String(lines));
  if (follow) {
    parts.push("-f");
  }
  return parts.join(" ");
}

function buildFallbackCommand(lines: number, follow: boolean): string {
  const tailFlag = follow ? "-f" : "";
  return [
    `tail ${tailFlag} -n ${lines} /var/log/syslog 2>/dev/null`,
    `tail ${tailFlag} -n ${lines} /var/log/messages 2>/dev/null`,
    `echo "No log source found"`,
  ].join(" || ");
}

async function streamVmLogs(
  vm: VmState,
  serviceName: string,
  color: string,
  lines: number,
  follow: boolean,
  timestamps: boolean,
  signal: AbortSignal,
): Promise<void> {
  const agent = createAgentClient(vm);

  const journalCmd = buildJournalctlCommand(lines, follow, timestamps);
  const fallbackCmd = buildFallbackCommand(lines, follow);
  const shellCmd = `${journalCmd} 2>/dev/null || ${fallbackCmd}`;

  const command = await agent.exec(
    { cmd: "/bin/sh", args: ["-c", shellCmd] },
    {
      signal,
      onStdout: (line: string) => {
        const prefix = `${color}[${serviceName}]${RESET}`;
        process.stdout.write(`${prefix} ${line}\n`);
      },
      onStderr: (line: string) => {
        const prefix = `${DIM}[${serviceName}]${RESET}`;
        process.stderr.write(`${prefix} ${line}\n`);
      },
    },
  );

  // If not following, wait for the command to finish.
  // If following, the command runs until aborted.
  await command.wait({ signal }).catch((err) => {
    if (toError(err).name === "AbortError") return;
    throw err;
  });
}

const appLogsCommand = defineCommand({
  meta: {
    name: "app",
    description: "Stream application logs from project VMs",
  },
  args: {
    service: {
      type: "positional",
      description: "Service name to filter (omit for all services)",
      required: false,
    },
    lines: {
      type: "string",
      alias: "n",
      default: "100",
      description: "Number of historical lines to show",
    },
    follow: {
      type: "boolean",
      alias: "f",
      default: true,
      description: "Follow log output (stream mode)",
    },
    timestamps: {
      type: "boolean",
      alias: "t",
      default: false,
      description: "Show timestamps in log output",
    },
    config: {
      type: "string",
      description: "Path to vmsan.toml (default: ./vmsan.toml)",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("logs:app");

    try {
      const numLines = Number.parseInt(args.lines, 10);
      if (Number.isNaN(numLines) || numLines < 0) {
        consola.error("--lines must be a non-negative integer");
        process.exitCode = 1;
        return;
      }

      // 1. Load project config
      const { project } = loadProjectConfig(args.config);

      // 2. Create VMService and find running VMs
      const vmService = await createVmsan();
      const projectVms = vmService
        .list()
        .filter((vm) => vm.project === project && vm.status === "running");

      if (projectVms.length === 0) {
        if (getOutputMode() === "json") {
          cmdLog.set({ project, error: "no_running_vms" });
          cmdLog.emit();
        } else {
          consola.warn(`No running VMs found for project "${project}"`);
          consola.info('Run "vmsan up" to start services.');
        }
        process.exitCode = 1;
        return;
      }

      // 3. Filter by service if specified
      let targetVms = projectVms;
      if (args.service) {
        targetVms = projectVms.filter((vm) => vm.network.service === args.service);
        if (targetVms.length === 0) {
          const available = projectVms
            .map((vm) => vm.network.service)
            .filter(Boolean)
            .join(", ");
          if (getOutputMode() === "json") {
            cmdLog.set({ project, service: args.service, error: "service_not_found" });
            cmdLog.emit();
          } else {
            consola.error(`No running VM found for service "${args.service}"`);
            if (available) {
              consola.info(`Available services: ${available}`);
            }
          }
          process.exitCode = 1;
          return;
        }
      }

      // 4. Assign colors to services
      const colorMap = new Map<string, string>();
      let colorIndex = 0;
      for (const vm of targetVms) {
        const name = vm.network.service ?? vm.id;
        if (!colorMap.has(name)) {
          colorMap.set(name, colorForIndex(colorIndex++));
        }
      }

      // 5. Set up abort controller for Ctrl+C cleanup
      const ac = new AbortController();
      const cleanup = () => ac.abort();
      process.on("SIGINT", cleanup);
      process.on("SIGTERM", cleanup);

      try {
        // 6. Start streaming from all target VMs concurrently
        if (!args.follow && getOutputMode() !== "json") {
          // In non-follow mode, show a brief header
          const serviceNames = targetVms.map((vm) => vm.network.service ?? vm.id);
          consola.info(`Showing last ${numLines} lines from: ${serviceNames.join(", ")}`);
          consola.log("");
        }

        const streams = targetVms.map((vm) => {
          const name = vm.network.service ?? vm.id;
          const color = colorMap.get(name)!;
          return streamVmLogs(vm, name, color, numLines, args.follow, args.timestamps, ac.signal);
        });

        await Promise.all(streams);
      } finally {
        process.off("SIGINT", cleanup);
        process.off("SIGTERM", cleanup);
      }

      cmdLog.set({
        project,
        service: args.service ?? "all",
        follow: args.follow,
        lines: numLines,
        vmCount: targetVms.length,
      });
      cmdLog.emit();
    } catch (error) {
      if (toError(error).name === "AbortError") return;
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default appLogsCommand as CommandDef;
