import { consola } from "consola";
import { EvlogError } from "evlog";
import type { CommandLogger } from "../lib/logger/index.ts";
import { getOutputMode } from "../lib/logger/index.ts";

export function handleCommandError(error: unknown, cmdLog: CommandLogger): void {
  cmdLog.error(error instanceof Error ? error : String(error));
  cmdLog.emit();

  // In JSON mode, cmdLog.emit() already produced structured error output.
  // Skip consola to avoid polluting stdout.
  if (getOutputMode() === "json") {
    return;
  }

  if (error instanceof EvlogError) {
    consola.error(error.message);
    if (error.why) consola.log(`  Why: ${error.why}`);
    if (error.fix) consola.log(`\n  Fix:\n\n${error.fix}\n`);
    if (error.link) consola.log(`  More: ${error.link}`);
  } else {
    consola.error(error instanceof Error ? error.message : String(error));
  }
}
