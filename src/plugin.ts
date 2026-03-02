import type { VmsanContext } from "./context.ts";

export interface VmsanPlugin {
  name: string;
  setup: (ctx: VmsanContext) => void | Promise<void>;
}

export function definePlugin(plugin: VmsanPlugin): VmsanPlugin {
  return plugin;
}
