import { join } from "node:path";
import type { VmStateStore } from "./vm-state.ts";
import type { VmsanPaths } from "../paths.ts";
import { spawnTimeoutKiller } from "./timeout-killer.ts";
import { safeKill } from "./utils.ts";

export interface TimeoutExtenderOptions {
  vmId: string;
  store: VmStateStore;
  paths: VmsanPaths;
  intervalMs?: number;
  signal?: AbortSignal;
}

const DEFAULT_INTERVAL_MS = 5 * 60 * 1000; // 5 minutes

export class TimeoutExtender {
  private _timer: ReturnType<typeof setInterval> | null = null;
  private _previousKillerPid: number | null = null;
  private readonly _vmId: string;
  private readonly _store: VmStateStore;
  private readonly _paths: VmsanPaths;
  private readonly _intervalMs: number;
  private readonly _signal?: AbortSignal;

  constructor(opts: TimeoutExtenderOptions) {
    this._vmId = opts.vmId;
    this._store = opts.store;
    this._paths = opts.paths;
    this._intervalMs = opts.intervalMs ?? DEFAULT_INTERVAL_MS;
    this._signal = opts.signal;
  }

  start(): void {
    if (this._timer) return;
    if (this._signal?.aborted) return;

    // Perform an immediate extension
    this._extend();

    this._timer = setInterval(() => this._extend(), this._intervalMs);

    if (this._signal) {
      this._signal.addEventListener("abort", () => this.stop(), { once: true });
    }
  }

  stop(): void {
    if (this._timer) {
      clearInterval(this._timer);
      this._timer = null;
    }

    // Kill any outstanding killer process
    if (this._previousKillerPid !== null) {
      safeKill(this._previousKillerPid);
      this._previousKillerPid = null;
    }
  }

  private _extend(): void {
    const state = this._store.load(this._vmId);
    if (!state || state.status !== "running" || !state.timeoutMs) return;

    const timeoutAt = new Date(Date.now() + state.timeoutMs).toISOString();
    this._store.update(this._vmId, { timeoutAt });

    // Kill previous killer before spawning a new one
    if (this._previousKillerPid !== null) {
      safeKill(this._previousKillerPid);
      this._previousKillerPid = null;
    }

    // Spawn a fresh detached bash timeout killer
    if (state.pid) {
      const killer = spawnTimeoutKiller({
        vmId: this._vmId,
        pid: state.pid,
        timeoutMs: state.timeoutMs,
        stateFile: join(this._paths.vmsDir, `${this._vmId}.json`),
      });
      this._previousKillerPid = killer.pid ?? null;
    }
  }
}
