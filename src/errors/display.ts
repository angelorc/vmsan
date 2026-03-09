import { consola } from "consola";
import { EvlogError } from "evlog";
import type { CommandLogger } from "../lib/logger/index.ts";
import { getOutputMode } from "../lib/logger/index.ts";
import { VmsanError } from "./base.ts";

export function handleCommandError(error: unknown, cmdLog: CommandLogger): void {
  cmdLog.error(error instanceof Error ? error : String(error));

  // evlog's error() only serializes a fixed set of fields (name, message, stack,
  // status, data, cause). Merge VmsanError-specific fields (code, fix) so they
  // appear at the top level of the error object in --json output.
  if (error instanceof VmsanError) {
    const { name: _n, message: _m, status: _s, data: _d, cause: _c, ...extra } = error.toJSON();
    if (Object.keys(extra).length > 0) {
      cmdLog.set({ error: extra });
    }
    // Promote fix/why to the error top level so consumers don't need to dig into data
    if (error.fix) cmdLog.set({ error: { fix: error.fix } });
    if (error.why) cmdLog.set({ error: { why: error.why } });
  }

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
