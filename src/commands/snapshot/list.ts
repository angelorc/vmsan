import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { handleCommandError } from "../../errors/index.ts";
import { createVmsanContext } from "../../context.ts";
import { SnapshotService, type SnapshotEntry } from "../../services/snapshot.ts";
import { table, timeAgo } from "../../lib/utils.ts";

const snapshotListCommand = defineCommand({
  meta: {
    name: "list",
    description: "List all snapshots",
  },
  async run() {
    const cmdLog = createCommandLogger("snapshot:list");
    const log = consola.withTag("snapshot");

    try {
      const ctx = createVmsanContext();
      const service = new SnapshotService(ctx);
      const snapshots = service.list();

      if (snapshots.length === 0) {
        if (getOutputMode() === "json") {
          cmdLog.set({ count: 0, snapshots: [] });
        } else {
          log.log("No snapshots found.");
          cmdLog.set({ count: 0 });
        }
        cmdLog.emit();
        return;
      }

      if (getOutputMode() === "json") {
        cmdLog.set({
          count: snapshots.length,
          snapshots: snapshots.map((s) => ({
            id: s.id,
            sizeMb: s.sizeMb,
            createdAt: s.createdAt,
          })),
        });
      } else {
        const output = table<SnapshotEntry>({
          rows: snapshots,
          columns: {
            ID: { value: (s) => s.id },
            SIZE: { value: (s) => `${s.sizeMb} MB` },
            CREATED: { value: (s) => timeAgo(s.createdAt) },
          },
        });

        log.log(output);
        cmdLog.set({ count: snapshots.length });
      }

      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default snapshotListCommand as CommandDef;
