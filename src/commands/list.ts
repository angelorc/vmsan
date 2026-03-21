import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { readFileSync } from "node:fs";
import { consola } from "consola";
import { createVmsan } from "../context.ts";
import { createCommandLogger, getOutputMode } from "../lib/logger/index.ts";
import { handleCommandError } from "../errors/index.ts";
import { table, timeAgo, timeRemaining } from "../lib/utils.ts";
import type { VmState } from "../lib/vm-state.ts";

const STATUS_COLORS: Record<string, string> = {
  running: "\x1b[36m", // cyan
  creating: "\x1b[35m", // magenta
  stopped: "\x1b[2;90m", // dim gray
  error: "\x1b[31m", // red
};
const RESET = "\x1b[0m";

function colorStatus(status: string): string {
  const color = STATUS_COLORS[status] || "";
  return `${color}${status}${RESET}`;
}

interface InterfaceStats {
  rxBytes: number;
  txBytes: number;
  rxPackets: number;
  txPackets: number;
}

function readStatFile(path: string): number {
  try {
    return Number.parseInt(readFileSync(path, "utf-8").trim(), 10) || 0;
  } catch {
    return 0;
  }
}

function getInterfaceStats(iface: string): InterfaceStats {
  const base = `/sys/class/net/${iface}/statistics`;
  return {
    rxBytes: readStatFile(`${base}/rx_bytes`),
    txBytes: readStatFile(`${base}/tx_bytes`),
    rxPackets: readStatFile(`${base}/rx_packets`),
    txPackets: readStatFile(`${base}/tx_packets`),
  };
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const index = Math.min(i, units.length - 1);
  const value = bytes / Math.pow(1024, index);
  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[index]}`;
}

function formatPackets(packets: number): string {
  if (packets === 0) return "0";
  if (packets < 1000) return `${packets}`;
  if (packets < 1_000_000) return `${(packets / 1000).toFixed(1)}k`;
  return `${(packets / 1_000_000).toFixed(1)}M`;
}

interface VmWithStats {
  vm: VmState;
  stats: InterfaceStats | null;
}

const listCommand = defineCommand({
  meta: {
    name: "list",
    description: "List all VMs",
  },
  args: {
    detailed: {
      type: "boolean",
      default: false,
      description: "Show per-VM traffic counters (rx/tx bytes and packets)",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("list");
    const log = consola.withTag("list");

    try {
      const vmsan = await createVmsan();
      const vms = vmsan.list();

      if (vms.length === 0) {
        if (getOutputMode() === "json") {
          cmdLog.set({ count: 0, vms: [] });
        } else {
          log.log("No VMs found.");
          cmdLog.set({ count: 0 });
        }
        cmdLog.emit();
        return;
      }

      const statuses: Record<string, number> = {};
      for (const vm of vms) {
        statuses[vm.status] = (statuses[vm.status] ?? 0) + 1;
      }

      consola.debug(
        `Found ${vms.length} VM(s): ${Object.entries(statuses)
          .map(([s, n]) => `${n} ${s}`)
          .join(", ")}`,
      );

      const detailed = args.detailed;

      // Gather stats for detailed mode
      const vmsWithStats: VmWithStats[] = vms.map((vm) => ({
        vm,
        stats: detailed && vm.status === "running" ? getInterfaceStats(vm.network.tapDevice) : null,
      }));

      if (getOutputMode() === "json") {
        cmdLog.set({
          count: vms.length,
          statuses,
          vms: vmsWithStats.map(({ vm, stats }) => ({
            id: vm.id,
            status: vm.status,
            createdAt: vm.createdAt,
            memSizeMib: vm.memSizeMib,
            vcpuCount: vm.vcpuCount,
            runtime: vm.runtime,
            timeoutAt: vm.timeoutAt ?? null,
            snapshot: vm.snapshot ?? null,
            tunnelHostnames: vm.network.tunnelHostnames ?? [],
            ...(detailed && stats
              ? {
                  traffic: {
                    rxBytes: stats.rxBytes,
                    txBytes: stats.txBytes,
                    rxPackets: stats.rxPackets,
                    txPackets: stats.txPackets,
                  },
                }
              : {}),
          })),
        });
      } else if (detailed) {
        const output = table<VmWithStats>({
          rows: vmsWithStats,
          columns: {
            ID: { value: ({ vm }) => vm.id },
            STATUS: {
              value: ({ vm }) => vm.status,
              color: ({ vm }) => colorStatus(vm.status),
            },
            CREATED: { value: ({ vm }) => timeAgo(vm.createdAt) },
            MEMORY: { value: ({ vm }) => `${vm.memSizeMib} MiB` },
            VCPUS: { value: ({ vm }) => vm.vcpuCount },
            RUNTIME: { value: ({ vm }) => vm.runtime },
            RX: {
              value: ({ stats }) => (stats ? formatBytes(stats.rxBytes) : "-"),
            },
            TX: {
              value: ({ stats }) => (stats ? formatBytes(stats.txBytes) : "-"),
            },
            "RX PKT": {
              value: ({ stats }) => (stats ? formatPackets(stats.rxPackets) : "-"),
            },
            "TX PKT": {
              value: ({ stats }) => (stats ? formatPackets(stats.txPackets) : "-"),
            },
            TIMEOUT: {
              value: ({ vm }) => (vm.timeoutAt ? timeRemaining(vm.timeoutAt) : "-"),
            },
            SNAPSHOT: { value: ({ vm }) => vm.snapshot ?? "-" },
          },
        });

        log.log(output);
        cmdLog.set({ count: vms.length, statuses });
      } else {
        const output = table<VmState>({
          rows: vms,
          columns: {
            ID: { value: (vm) => vm.id },
            STATUS: {
              value: (vm) => vm.status,
              color: (vm) => colorStatus(vm.status),
            },
            CREATED: { value: (vm) => timeAgo(vm.createdAt) },
            MEMORY: { value: (vm) => `${vm.memSizeMib} MiB` },
            VCPUS: { value: (vm) => vm.vcpuCount },
            RUNTIME: { value: (vm) => vm.runtime },
            TIMEOUT: {
              value: (vm) => (vm.timeoutAt ? timeRemaining(vm.timeoutAt) : "-"),
            },
            TUNNEL: {
              value: (vm) =>
                vm.network.tunnelHostnames?.length
                  ? vm.network.tunnelHostnames.map((h) => `https://${h}`).join(", ")
                  : vm.network.tunnelHostname
                    ? `https://${vm.network.tunnelHostname}`
                    : "-",
            },
            SNAPSHOT: { value: (vm) => vm.snapshot ?? "-" },
          },
        });

        log.log(output);
        cmdLog.set({ count: vms.length, statuses });
      }
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default listCommand as CommandDef;
