import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { readdirSync, readFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import { consola } from "consola";
import { vmsanPaths } from "../../paths.ts";
import { SqliteVmStateStore } from "../../lib/state/sqlite.ts";
import { handleCommandError } from "../../errors/index.ts";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { toError } from "../../lib/utils.ts";
import type { VmState } from "../../lib/vm-state.ts";

function discoverJsonStates(vmsDir: string): { id: string; state: VmState }[] {
  if (!existsSync(vmsDir)) return [];
  const files = readdirSync(vmsDir).filter((f) => f.endsWith(".json"));
  const results: { id: string; state: VmState }[] = [];

  for (const file of files) {
    const id = file.replace(/\.json$/, "");
    try {
      const raw = readFileSync(join(vmsDir, file), "utf-8");
      const state = JSON.parse(raw) as VmState;
      results.push({ id, state });
    } catch (err) {
      consola.warn(`Skipping ${file}: ${toError(err).message}`);
    }
  }

  return results;
}

const migrateCommand = defineCommand({
  meta: {
    name: "migrate",
    description: "Migrate JSON state files to SQLite",
  },
  args: {
    "dry-run": {
      type: "boolean",
      default: false,
      description: "Show what would be imported without making changes",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("migrate");

    try {
      const paths = vmsanPaths();
      const jsonStates = discoverJsonStates(paths.vmsDir);

      if (jsonStates.length === 0) {
        consola.info("No JSON state files found in " + paths.vmsDir);
        cmdLog.set({ imported: 0, skipped: 0, total: 0 });
        cmdLog.emit();
        return;
      }

      consola.info(`Found ${jsonStates.length} JSON state file(s) in ${paths.vmsDir}`);

      if (getOutputMode() !== "json") {
        for (const { id, state } of jsonStates) {
          consola.log(`  ${id} — ${state.status} (project: ${state.project || "-"})`);
        }
        consola.log("");
      }

      if (args["dry-run"]) {
        consola.info("Dry run — no changes made");
        cmdLog.set({
          dryRun: true,
          total: jsonStates.length,
          vms: jsonStates.map(({ id, state }) => ({
            id,
            status: state.status,
            project: state.project,
          })),
        });
        cmdLog.emit();
        return;
      }

      // Prompt for confirmation
      const answer = await consola.prompt(
        "Back up ~/.vmsan/vms before proceeding. Continue? [y/N]",
        { type: "confirm", initial: false },
      );
      if (!answer) {
        consola.info("Migration cancelled");
        return;
      }

      // Open SQLite store
      const dbPath = join(paths.baseDir, "state.db");
      const store = new SqliteVmStateStore(dbPath);

      let imported = 0;
      let skipped = 0;

      for (const { id, state } of jsonStates) {
        // Check if already exists in SQLite (idempotent)
        const existing = store.load(id);
        if (existing) {
          consola.debug(`Skipping ${id} — already in SQLite`);
          skipped++;
          continue;
        }

        try {
          store.save(state);
          imported++;
          consola.success(`Imported ${id}`);
        } catch (err) {
          consola.error(`Failed to import ${id}: ${toError(err).message}`);
          skipped++;
        }
      }

      // Verify count
      const sqliteCount = store.vmCount();
      consola.log("");
      consola.info(`SQLite now has ${sqliteCount} VM(s)`);

      if (imported > 0) {
        consola.success(`Migration complete: ${imported} imported, ${skipped} skipped`);
      } else {
        consola.info(`Nothing to import (${skipped} already in SQLite)`);
      }

      consola.info("JSON files left in place — remove manually after verifying");

      store.close();

      cmdLog.set({ imported, skipped, total: jsonStates.length, sqliteCount });
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default migrateCommand as CommandDef;
