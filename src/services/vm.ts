import { dirname } from "node:path";
import { rmSync } from "node:fs";
import type { VmsanPaths } from "../paths.ts";
import { FileVmStateStore, type VmState } from "../lib/vm-state.ts";
import { NetworkManager, type NetworkConfig } from "../lib/network.ts";
import { getVmJailerPid, getVmPid } from "../commands/create/environment.ts";
import { VmsanError, vmNotFoundError, vmNotStoppedError } from "../errors/index.ts";
import { safeKill } from "../lib/utils.ts";

export interface StopResult {
  vmId: string;
  success: boolean;
  error?: VmsanError;
  alreadyStopped?: boolean;
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

      // 2. Teardown networking
      if (state.network) {
        const networkConfig: NetworkConfig = {
          slot: Number(state.network.hostIp.split(".")[2]),
          tapDevice: state.network.tapDevice,
          hostIp: state.network.hostIp,
          guestIp: state.network.guestIp,
          subnetMask: state.network.subnetMask,
          macAddress: state.network.macAddress,
          networkPolicy: state.network.networkPolicy,
          allowedDomains: state.network.allowedDomains,
          allowedCidrs: state.network.allowedCidrs || [],
          deniedCidrs: state.network.deniedCidrs || [],
          publishedPorts: state.network.publishedPorts,
        };
        try {
          NetworkManager.fromConfig(networkConfig).teardown();
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
