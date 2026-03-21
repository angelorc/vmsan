import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { basename, resolve } from "node:path";
import { existsSync } from "node:fs";
import { consola } from "consola";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { handleCommandError } from "../../errors/index.ts";
import { SecretsStore } from "../../lib/secrets/index.ts";

function resolveProject(): string {
  const cwd = process.cwd();
  const tomlPath = resolve(cwd, "vmsan.toml");
  if (existsSync(tomlPath)) {
    return basename(cwd);
  }
  return basename(cwd);
}

const secretsListCommand = defineCommand({
  meta: {
    name: "list",
    description: "List secret names for the current project",
  },
  async run() {
    const cmdLog = createCommandLogger("secrets:list");

    try {
      const project = resolveProject();
      const store = new SecretsStore();
      const keys = store.list(project);

      if (getOutputMode() === "json") {
        cmdLog.set({ project, count: keys.length, keys });
      } else {
        if (keys.length === 0) {
          consola.info(`No secrets for project "${project}"`);
        } else {
          consola.info(`Secrets for project "${project}":`);
          for (const key of keys) {
            consola.log(`  ${key}`);
          }
        }
      }

      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default secretsListCommand as CommandDef;
