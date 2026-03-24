import { createHooks, type Hookable } from "hookable";
import { vmsanPaths, type VmsanPaths } from "./paths.ts";
import type { VmStateStore } from "./lib/vm-state.ts";
import { createStateStore } from "./lib/state/index.ts";
import type { VmsanHooks } from "./hooks.ts";
import type { VmsanPlugin } from "./plugin.ts";
import { createDefaultLogger, type VmsanLogger } from "./vmsan-logger.ts";
import { cloudflarePlugin } from "./plugins/cloudflare.ts";

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

function buildContext(options?: VmsanOptions): VmsanContext {
  const paths =
    options?.paths === undefined
      ? vmsanPaths()
      : typeof options.paths === "string"
        ? vmsanPaths(options.paths)
        : options.paths;

  const store = options?.store ?? createStateStore(paths.vmsDir);
  const logger = options?.logger ?? createDefaultLogger();
  const hooks = createHooks<VmsanHooks>();

  return { paths, store, hooks, logger };
}

export function createVmsanContext(options?: VmsanOptions): VmsanContext {
  return buildContext(options);
}

export async function createVmsan(options?: VmsanOptions): Promise<VmsanContext> {
  const ctx = buildContext(options);

  const builtinPlugins: VmsanPlugin[] = [cloudflarePlugin(ctx.paths.baseDir)];
  const allPlugins = [...builtinPlugins, ...(options?.plugins ?? [])];
  for (const plugin of allPlugins) {
    await plugin.setup(ctx);
  }

  return ctx;
}
