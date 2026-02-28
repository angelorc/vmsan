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
    const parts = [error.message];
    if (error.why) parts.push(`  Why: ${error.why}`);
    if (error.fix) parts.push(`  Fix: ${error.fix}`);
    if (error.link) parts.push(`  More: ${error.link}`);
    consola.error(parts.join("\n"));
  } else {
    consola.error(error instanceof Error ? error.message : String(error));
  }
}
