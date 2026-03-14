import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createCommandLogger } from "../../lib/logger/index.ts";
import { handleCommandError } from "../../errors/index.ts";
import { createVmsanContext } from "../../context.ts";
import { SnapshotService } from "../../services/snapshot.ts";
import { toError } from "../../lib/utils.ts";

const snapshotDeleteCommand = defineCommand({
  meta: {
    name: "delete",
    description: "Delete one or more snapshots",
  },
  args: {
    snapshotIds: {
      type: "positional",
      description: "Snapshot IDs to delete",
      required: true,
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("snapshot:delete");

    try {
      const rawIds = [args.snapshotIds, ...args._];
      const snapshotIds = Array.from(new Set(rawIds.filter(Boolean)));

      if (snapshotIds.length === 0) {
        consola.error("No snapshot IDs provided.");
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      const ctx = createVmsanContext();
      const service = new SnapshotService(ctx);

      const results: { snapshotId: string; success: boolean }[] = [];
      let hasErrors = false;

      for (const id of snapshotIds) {
        const log = consola.withTag(id);
        try {
          service.delete(id);
          log.success(`Deleted ${id}`);
          results.push({ snapshotId: id, success: true });
        } catch (err) {
          log.error(`Failed to delete ${id}: ${toError(err).message}`);
          results.push({ snapshotId: id, success: false });
          hasErrors = true;
        }
      }

      cmdLog.set({ snapshotIds, results });
      if (hasErrors) {
        process.exitCode = 1;
      }
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default snapshotDeleteCommand as CommandDef;
