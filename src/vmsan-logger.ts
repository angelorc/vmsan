import { consola, type ConsolaInstance } from "consola";

export interface VmsanLogger {
  debug: (...args: unknown[]) => void;
  info: (...args: unknown[]) => void;
  success: (...args: unknown[]) => void;
  warn: (...args: unknown[]) => void;
  error: (...args: unknown[]) => void;
  start: (...args: unknown[]) => void;
  box: (message: string) => void;
  withTag: (tag: string) => VmsanLogger;
}

export function createDefaultLogger(): VmsanLogger {
  return wrapConsola(consola);
}

function wrapConsola(instance: ConsolaInstance): VmsanLogger {
  return {
    debug: instance.debug.bind(instance),
    info: instance.info.bind(instance),
    success: instance.success.bind(instance),
    warn: instance.warn.bind(instance),
    error: instance.error.bind(instance),
    start: instance.start.bind(instance),
    box: (message: string) => instance.box(message),
    withTag: (tag: string) => wrapConsola(instance.withTag(tag)),
  };
}

const noop = (): void => {};

export function createSilentLogger(): VmsanLogger {
  const silent: VmsanLogger = {
    debug: noop,
    info: noop,
    success: noop,
    warn: noop,
    error: noop,
    start: noop,
    box: noop,
    withTag: () => silent,
  };
  return silent;
}
