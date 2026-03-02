import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createCommandLogger } from "../lib/logger/index.ts";
import { createVmsan } from "../context.ts";

const stopCommand = defineCommand({
  meta: {
    name: "stop",
    description: "Stop one or more running VMs",
  },
  args: {
    vmIds: {
      type: "positional",
      description: "VM IDs to stop",
      required: true,
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("stop");
    const rawIds = [args.vmIds, ...args._];
    const vmIds = Array.from(new Set(rawIds.filter(Boolean)));

    if (vmIds.length === 0) {
      consola.error("No VM IDs provided.");
      cmdLog.emit();
      process.exitCode = 1;
      return;
    }

    const vmsan = await createVmsan();

    // Validate all IDs exist upfront
    const missing: string[] = [];
    for (const id of vmIds) {
      if (!vmsan.get(id)) missing.push(id);
    }
    if (missing.length > 0) {
      consola.error(`VM(s) not found: ${missing.join(", ")}`);
      cmdLog.emit();
      process.exitCode = 1;
      return;
    }

    // Stop sequentially (shared iptables resources)
    const results: { vmId: string; success: boolean }[] = [];
    let hasErrors = false;
    for (const id of vmIds) {
      const log = consola.withTag(id);
      const vm = vmsan.get(id);
      consola.debug(`VM ${id} current status: ${vm?.status ?? "unknown"}`);

      log.start(`Stopping ${id}...`);
      const result = await vmsan.stop(id);

      if (result.alreadyStopped) {
        log.warn(`${id} is already stopped`);
        results.push({ vmId: id, success: true });
      } else if (result.success) {
        log.success(`Stopped ${id}`);
        results.push({ vmId: id, success: true });
      } else {
        consola.debug(`Stop failed for ${id}: ${result.error?.message ?? "unknown"}`);
        log.error(`Failed to stop ${id}: ${result.error?.message ?? "unknown error"}`);
        results.push({ vmId: id, success: false });
        hasErrors = true;
      }
    }

    cmdLog.set({ vmIds, results });
    if (hasErrors) {
      process.exitCode = 1;
    }
    cmdLog.emit();
  },
});

export default stopCommand as CommandDef;
