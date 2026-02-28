import { dirname, join } from "node:path";
import { rmSync } from "node:fs";
import type { VmsanPaths } from "../paths.ts";
import { FileVmStateStore, type VmState } from "../lib/vm-state.ts";
import { NetworkManager } from "../lib/network.ts";
import { FileLock } from "../lib/file-lock.ts";
import { getVmJailerPid, getVmPid } from "../commands/create/environment.ts";
import {
  VmsanError,
  vmNotFoundError,
  vmNotStoppedError,
  vmNotRunningError,
} from "../errors/index.ts";
import { safeKill } from "../lib/utils.ts";

export interface StopResult {
  vmId: string;
  success: boolean;
  error?: VmsanError;
  alreadyStopped?: boolean;
}

export interface UpdatePolicyResult {
  vmId: string;
  success: boolean;
  previousPolicy: string;
  newPolicy: string;
  error?: VmsanError;
}

export class VMService {
  protected store: FileVmStateStore;

  constructor(protected readonly paths: VmsanPaths) {
    this.store = new FileVmStateStore(paths.vmsDir);
  }

  list(): VmState[] {
    return this.store
      .list()
      .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());
  }

  get(vmId: string): VmState | null {
    return this.store.load(vmId);
  }

  async stop(vmId: string): Promise<StopResult> {
    const state = this.store.load(vmId);
    if (!state) {
      return { vmId, success: false, error: vmNotFoundError(vmId) };
    }

    if (state.status === "stopped") {
      return { vmId, success: true, alreadyStopped: true };
    }

    try {
      // 1. Kill process — stored PID first, then /proc scan fallback
      if (state.pid) {
        safeKill(state.pid, "SIGKILL");
      }
      const orphanPid = getVmPid(vmId);
      if (orphanPid) {
        safeKill(orphanPid, "SIGKILL");
      }
      const orphanJailerPid = getVmJailerPid(vmId);
      if (orphanJailerPid) {
        safeKill(orphanJailerPid, "SIGKILL");
      }

      // 2. Teardown networking (including namespace if present)
      if (state.network) {
        try {
          NetworkManager.fromVmNetwork(state.network).teardown();
        } catch {
          // Network may be partially torn down from a prior attempt
        }
      }

      // 3. Update state
      this.store.update(vmId, { status: "stopped", pid: null });

      return { vmId, success: true };
    } catch (err) {
      return { vmId, success: false, error: err instanceof VmsanError ? err : undefined };
    }
  }

  async updateNetworkPolicy(
    vmId: string,
    policy: string,
    domains: string[],
    allowedCidrs: string[],
    deniedCidrs: string[],
  ): Promise<UpdatePolicyResult> {
    const stateFile = join(this.paths.vmsDir, `${vmId}.json`);
    const fileLock = new FileLock(stateFile, `update-policy-${vmId}`);

    return fileLock.run(() => {
      const state = this.store.load(vmId);
      if (!state) {
        return {
          vmId,
          success: false,
          previousPolicy: "",
          newPolicy: policy,
          error: vmNotFoundError(vmId),
        };
      }

      if (state.status !== "running") {
        return {
          vmId,
          success: false,
          previousPolicy: state.network.networkPolicy,
          newPolicy: policy,
          error: vmNotRunningError(vmId),
        };
      }

      const previousPolicy = state.network.networkPolicy;

      try {
        const mgr = NetworkManager.fromVmNetwork(state.network);
        mgr.updatePolicy(policy, domains, allowedCidrs, deniedCidrs);

        this.store.update(vmId, {
          network: {
            ...state.network,
            networkPolicy: policy,
            allowedDomains: domains,
            allowedCidrs,
            deniedCidrs,
          },
        });

        return { vmId, success: true, previousPolicy, newPolicy: policy };
      } catch (err) {
        return {
          vmId,
          success: false,
          previousPolicy,
          newPolicy: policy,
          error: err instanceof VmsanError ? err : undefined,
        };
      }
    });
  }

  async remove(vmId: string, opts?: { force?: boolean }): Promise<StopResult> {
    const state = this.store.load(vmId);
    if (!state) {
      return { vmId, success: false, error: vmNotFoundError(vmId) };
    }

    try {
      // 1. Stop if running (only when --force is used; caller should pre-check)
      if (state.status !== "stopped") {
        if (!opts?.force) {
          return {
            vmId,
            success: false,
            error: vmNotStoppedError(vmId, state.status),
          };
        }
        const stopResult = await this.stop(vmId);
        if (!stopResult.success) {
          return stopResult;
        }
      }

      // 2. Remove chroot dir + parent jailer dir
      if (state.chrootDir) {
        const vmJailerDir = dirname(state.chrootDir);
        try {
          rmSync(state.chrootDir, { recursive: true, force: true });
        } catch {
          // Directory may be busy or mounted
        }
        try {
          rmSync(vmJailerDir, { recursive: true, force: true });
        } catch {
          // Parent directory may be busy or already removed
        }
      }

      // 3. Delete state file — VM disappears from list
      this.store.delete(vmId);

      return { vmId, success: true };
    } catch (err) {
      return { vmId, success: false, error: err instanceof VmsanError ? err : undefined };
    }
  }
}
