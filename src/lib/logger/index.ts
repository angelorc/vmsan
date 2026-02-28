import { consola, type ConsolaInstance } from "consola";
import { createRequestLogger, initLogger } from "evlog";
import type { RequestLogger } from "evlog";

export type OutputMode = "normal" | "json" | "verbose" | "silent";

let currentMode: OutputMode = "normal";

/**
 * Initialize both consola and evlog for the given output mode.
 * Call once per CLI invocation, before any command runs.
 *
 * consola = human-facing CLI output (icons, colors, boxes, progress)
 * evlog   = machine-readable structured events (--json output)
 */
export function initVmsanLogger(mode: OutputMode): void {
  currentMode = mode;

  switch (mode) {
    case "normal":
      // Fancy consola output, evlog disabled (no structured output)
      initLogger({ enabled: false, env: { service: "vmsan" } });
      break;

    case "json":
      // Silence consola entirely, evlog emits JSON lines
      consola.level = -999;
      initLogger({ enabled: true, pretty: false, stringify: true, env: { service: "vmsan" } });
      break;

    case "verbose":
      // Fancy consola with debug-level visible, evlog pretty-prints tree at end
      consola.level = 4;
      consola.options.formatOptions = {
        ...consola.options.formatOptions,
        date: true,
      };
      initLogger({ enabled: true, pretty: true, stringify: false, env: { service: "vmsan" } });
      break;

    case "silent":
      consola.level = -999;
      initLogger({ enabled: false, env: { service: "vmsan" } });
      break;
  }
}

export function getOutputMode(): OutputMode {
  return currentMode;
}

export interface CommandLogger {
  /** Add structured context to the wide event */
  set: RequestLogger["set"];
  /** Record an error in the wide event */
  error: RequestLogger["error"];
  /** Emit the wide event (only produces output in json/verbose modes) */
  emit: () => void;
}

/**
 * Create a command-scoped logger backed by an evlog RequestLogger.
 * The wide event is emitted only in json/verbose modes.
 */
export function createCommandLogger(command: string): CommandLogger {
  const reqLog = createRequestLogger({ path: command });

  return {
    set: reqLog.set.bind(reqLog),
    error: reqLog.error.bind(reqLog),
    emit: () => {
      if (currentMode === "json" || currentMode === "verbose") {
        reqLog.emit();
      }
    },
  };
}

/**
 * Create a scoped consola instance with a tag prefix.
 * Output shows as `[tag] message`.
 */
export function createScopedLogger(tag: string): ConsolaInstance {
  return consola.withTag(tag);
}
