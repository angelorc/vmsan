import type { CommandDef } from "citty";
import { defineCommand } from "citty";
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

const listCommand = defineCommand({
  meta: {
    name: "list",
    description: "List all VMs",
  },
  async run() {
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

      if (getOutputMode() === "json") {
        cmdLog.set({
          count: vms.length,
          statuses,
          vms: vms.map((vm) => ({
            id: vm.id,
            status: vm.status,
            createdAt: vm.createdAt,
            memSizeMib: vm.memSizeMib,
            vcpuCount: vm.vcpuCount,
            runtime: vm.runtime,
            timeoutAt: vm.timeoutAt ?? null,
            snapshot: vm.snapshot ?? null,
          })),
        });
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
