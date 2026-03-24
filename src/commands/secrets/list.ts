import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { handleCommandError } from "../../errors/index.ts";
import { SecretsStore } from "../../lib/secrets/index.ts";
import { resolveProject } from "./resolve-project.ts";

const secretsListCommand = defineCommand({
  meta: {
    name: "list",
    description: "List secret names for the current project",
  },
  args: {
    project: {
      type: "string",
      alias: "p",
      description: "Project name (default: auto-detect from vmsan.toml or directory name)",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("secrets:list");

    try {
      const project = resolveProject(args.project);
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
