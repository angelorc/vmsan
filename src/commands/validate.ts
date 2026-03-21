import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { consola } from "consola";
import { parseTomlSafe, validateToml } from "../lib/toml/validate.ts";
import { createCommandLogger, getOutputMode } from "../lib/logger/index.ts";
import { handleCommandError } from "../errors/index.ts";

const PASS = "\x1b[32m\u2713\x1b[0m";
const FAIL = "\x1b[31m\u2717\x1b[0m";

const validateCommand = defineCommand({
  meta: {
    name: "validate",
    description: "Validate a vmsan.toml configuration file",
  },
  args: {
    path: {
      type: "positional",
      required: false,
      description: "Path to vmsan.toml (defaults to ./vmsan.toml)",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("validate");

    try {
      const filePath = resolve(args.path || "vmsan.toml");

      if (!existsSync(filePath)) {
        consola.error(`File not found: ${filePath}`);
        cmdLog.set({ valid: false, error: "File not found" });
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      const tomlText = readFileSync(filePath, "utf-8");

      // Parse
      const { config, errors: syntaxErrors } = parseTomlSafe(tomlText);

      if (syntaxErrors.length > 0) {
        if (getOutputMode() === "json") {
          cmdLog.set({ valid: false, errors: syntaxErrors });
        } else {
          for (const err of syntaxErrors) {
            const lineInfo = err.line ? ` (line ${err.line})` : "";
            consola.log(`${FAIL} Syntax error${lineInfo}: ${err.message}`);
          }
        }
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      if (getOutputMode() !== "json") {
        consola.log(`${PASS} Syntax valid`);
      }

      // Validate
      const errors = validateToml(config!);

      if (errors.length === 0) {
        if (getOutputMode() === "json") {
          cmdLog.set({ valid: true, errors: [] });
        } else {
          consola.log(`${PASS} Dependencies valid`);
          consola.log(`${PASS} Ports valid`);
          consola.log(`${PASS} Runtimes valid`);
        }
        cmdLog.emit();
        return;
      }

      // Report errors
      const hasDependencyErrors = errors.some((e) => e.field.includes("depends_on"));
      const hasPortErrors = errors.some((e) => e.field.includes("connect_to"));
      const hasRuntimeErrors = errors.some((e) => e.field.includes("runtime"));

      if (getOutputMode() === "json") {
        cmdLog.set({ valid: false, errors });
      } else {
        if (!hasDependencyErrors) {
          consola.log(`${PASS} Dependencies valid`);
        }
        if (!hasPortErrors) {
          consola.log(`${PASS} Ports valid`);
        }
        if (!hasRuntimeErrors) {
          consola.log(`${PASS} Runtimes valid`);
        }

        for (const err of errors) {
          const lineInfo = err.line ? ` (line ${err.line})` : "";
          consola.log(`${FAIL} Error${lineInfo}: ${err.message}`);
          if (err.suggestion) {
            consola.log(`  Suggestion: ${err.suggestion}`);
          }
        }
      }

      cmdLog.emit();
      process.exitCode = 1;
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default validateCommand as CommandDef;
