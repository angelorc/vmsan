import { createHooks, type Hookable } from "hookable";
import { vmsanPaths, type VmsanPaths } from "./paths.ts";
import { FileVmStateStore, type VmStateStore } from "./lib/vm-state.ts";
import type { VmsanHooks } from "./hooks.ts";
import type { VmsanPlugin } from "./plugin.ts";
import { createDefaultLogger, type VmsanLogger } from "./vmsan-logger.ts";
import { VMService } from "./services/vm.ts";

export interface VmsanOptions {
  paths?: string | VmsanPaths;
  store?: VmStateStore;
  logger?: VmsanLogger;
  plugins?: VmsanPlugin[];
}

export interface VmsanContext {
  readonly paths: VmsanPaths;
  readonly store: VmStateStore;
  readonly hooks: Hookable<VmsanHooks>;
  readonly logger: VmsanLogger;
}

export async function createVmsan(options?: VmsanOptions): Promise<VMService> {
  const paths =
    options?.paths === undefined
      ? vmsanPaths()
      : typeof options.paths === "string"
        ? vmsanPaths(options.paths)
        : options.paths;

  const store = options?.store ?? new FileVmStateStore(paths.vmsDir);
  const logger = options?.logger ?? createDefaultLogger();
  const hooks = createHooks<VmsanHooks>();

  const ctx: VmsanContext = { paths, store, hooks, logger };
  const vmsan = new VMService(ctx);

  if (options?.plugins) {
    for (const plugin of options.plugins) {
      await plugin.setup(ctx);
    }
  }

  return vmsan;
}
