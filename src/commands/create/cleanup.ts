import { dirname } from "node:path";
import { rmSync } from "node:fs";
import type { VmsanPaths } from "../../paths.ts";
import { FileVmStateStore } from "../../lib/vm-state.ts";
import { NetworkManager, type NetworkConfig } from "../../lib/network.ts";
import { safeKill } from "../../lib/utils.ts";
import { getVmJailerPid, getVmPid } from "./environment.ts";

export function killOrphanVmProcess(vmId: string): void {
  const orphanPid = getVmPid(vmId);
  const orphanJailerPid = getVmJailerPid(vmId);

  if (orphanPid) {
    safeKill(orphanPid, "SIGKILL");
  }

  if (orphanJailerPid) {
    safeKill(orphanJailerPid, "SIGKILL");
  }
}

export function markVmAsError(vmId: string, error: unknown, paths: VmsanPaths): void {
  try {
    const store = new FileVmStateStore(paths.vmsDir);
    store.update(vmId, {
      status: "error",
      error: error instanceof Error ? error.message : String(error),
    });
  } catch {
    // State store may be corrupt during error recovery
  }
}

export function cleanupNetwork(networkConfig: NetworkConfig | undefined): void {
  if (!networkConfig) return;
  try {
    NetworkManager.fromConfig(networkConfig).teardown();
  } catch {
    // Network may be partially torn down from a prior attempt
  }
}

export function cleanupChroot(chrootDir: string | undefined): void {
  if (!chrootDir) return;

  const vmJailerDir = dirname(chrootDir);
  try {
    rmSync(chrootDir, { recursive: true, force: true });
  } catch {
    // Directory may be busy or mounted
  }
  try {
    rmSync(vmJailerDir, { recursive: true, force: true });
  } catch {
    // Parent directory may be busy or already removed
  }
}
