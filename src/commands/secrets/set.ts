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

const secretsSetCommand = defineCommand({
  meta: {
    name: "set",
    description: "Set a secret (KEY=VALUE or KEY VALUE)",
  },
  args: {
    keyValue: {
      type: "positional",
      description: "Secret in KEY=VALUE format, or KEY followed by VALUE",
      required: true,
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("secrets:set");

    try {
      let key: string;
      let value: string;

      const raw = args.keyValue;
      const eqIndex = raw.indexOf("=");

      if (eqIndex > 0) {
        // KEY=VALUE format
        key = raw.slice(0, eqIndex);
        value = raw.slice(eqIndex + 1);
      } else if (args._.length > 0) {
        // KEY VALUE format (positional + extra args)
        key = raw;
        value = args._[0];
      } else {
        consola.error(
          "Invalid format. Use: vmsan secrets set KEY=VALUE or vmsan secrets set KEY VALUE",
        );
        process.exitCode = 1;
        return;
      }

      if (!key) {
        consola.error("Secret key cannot be empty.");
        process.exitCode = 1;
        return;
      }

      const project = resolveProject();
      const store = new SecretsStore();
      store.set(project, key, value);

      if (getOutputMode() === "json") {
        cmdLog.set({ project, key });
      } else {
        consola.success(`Secret "${key}" set for project "${project}"`);
      }

      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default secretsSetCommand as CommandDef;
