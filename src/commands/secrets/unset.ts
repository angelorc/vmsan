import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { handleCommandError } from "../../errors/index.ts";
import { SecretsStore } from "../../lib/secrets/index.ts";
import { resolveProject } from "./resolve-project.ts";

const secretsUnsetCommand = defineCommand({
  meta: {
    name: "unset",
    description: "Remove a secret by key name",
  },
  args: {
    key: {
      type: "positional",
      description: "Secret key to remove",
      required: true,
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("secrets:unset");

    try {
      const project = resolveProject();
      const store = new SecretsStore();
      const removed = store.unset(project, args.key);

      if (getOutputMode() === "json") {
        cmdLog.set({ project, key: args.key, removed });
      } else {
        if (removed) {
          consola.success(`Secret "${args.key}" removed from project "${project}"`);
        } else {
          consola.warn(`Secret "${args.key}" not found in project "${project}"`);
        }
      }

      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default secretsUnsetCommand as CommandDef;
