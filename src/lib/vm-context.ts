import { FileVmStateStore } from "./vm-state.ts";
import type { VmState, VmStateStore } from "./vm-state.ts";
import type { VmsanPaths } from "../paths.ts";
import { vmNotFoundError, vmNotRunningError, vmNoAgentTokenError } from "../errors/index.ts";
import { agentTimeoutError } from "../errors/index.ts";
import { createStateStore } from "./state/index.ts";

export interface RunningVmContext {
  state: VmState & { agentToken: string };
  guestIp: string;
  port: number;
  store: VmStateStore;
}

/**
 * Load VM state and validate it's running with an agent token.
 * Throws VmError on failure (handled by handleCommandError in the caller).
 */
export function resolveVmState(vmId: string, paths: VmsanPaths): RunningVmContext {
  const store = createStateStore(paths.vmsDir);
  const state = store.load(vmId);
  if (!state) throw vmNotFoundError(vmId);
  if (state.status !== "running") throw vmNotRunningError(vmId, state.status);
  if (!state.agentToken) throw vmNoAgentTokenError(vmId);
  return {
    state: state as VmState & { agentToken: string },
    guestIp: state.network.guestIp,
    port: state.agentPort || paths.agentPort,
    store,
  };
}

/**
 * Poll the agent health endpoint until it responds OK.
 */
export async function waitForAgent(
  guestIp: string,
  port: number,
  timeoutMs = 60_000,
): Promise<void> {
  const start = Date.now();
  const url = `http://${guestIp}:${port}/health`;
  while (Date.now() - start < timeoutMs) {
    try {
      const res = await fetch(url, { signal: AbortSignal.timeout(2000) });
      if (res.ok) return;
    } catch {
      // Agent not ready yet
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw agentTimeoutError(guestIp, timeoutMs);
}
