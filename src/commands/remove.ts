import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { vmsanPaths } from "../paths.ts";
import { VMService } from "../services/vm.ts";
import { createCommandLogger } from "../lib/logger/index.ts";

const removeCommand = defineCommand({
  meta: {
    name: "remove",
    description: "Remove one or more VMs (stops if running, then deletes)",
  },
  args: {
    vmIds: {
      type: "positional",
      description: "VM IDs to remove",
      required: true,
    },
    force: {
      type: "boolean",
      alias: "f",
      default: false,
      description: "Force removal of running VMs (stops them first)",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("remove");
    const rawIds = [args.vmIds, ...args._];
    const vmIds = Array.from(new Set(rawIds.filter(Boolean)));

    if (vmIds.length === 0) {
      consola.error("No VM IDs provided.");
      cmdLog.emit();
      process.exitCode = 1;
      return;
    }

    const service = new VMService(vmsanPaths());

    // Validate all IDs exist upfront
    const missing: string[] = [];
    for (const id of vmIds) {
      if (!service.get(id)) missing.push(id);
    }
    if (missing.length > 0) {
      consola.error(`VM(s) not found: ${missing.join(", ")}`);
      cmdLog.emit();
      process.exitCode = 1;
      return;
    }

    // Block removal of non-stopped VMs unless --force is used
    if (!args.force) {
      const running: string[] = [];
      for (const id of vmIds) {
        const vm = service.get(id)!;
        if (vm.status !== "stopped") running.push(id);
      }
      if (running.length > 0) {
        consola.error(
          `Cannot remove running VM(s): ${running.join(", ")}. Stop them first or use --force (-f).`,
        );
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }
    }

    // Remove sequentially
    const results: { vmId: string; success: boolean }[] = [];
    let hasErrors = false;
    for (const id of vmIds) {
      const log = consola.withTag(id);
      const vm = service.get(id)!;
      consola.debug(`VM ${id} status=${vm.status}, force=${args.force}`);

      log.start(`Removing ${id}...`);
      const result = await service.remove(id, { force: args.force });

      if (result.success) {
        log.success(`Removed ${id}`);
        results.push({ vmId: id, success: true });
      } else {
        consola.debug(`Removal failed for ${id}: ${result.error?.message ?? "unknown"}`);
        log.error(`Failed to remove ${id}: ${result.error?.message ?? "unknown error"}`);
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

export default removeCommand as CommandDef;
