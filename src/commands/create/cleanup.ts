import { dirname } from "node:path";
import { rmSync } from "node:fs";
import { consola } from "consola";
import type { VmsanPaths } from "../../paths.ts";
import { FileVmStateStore } from "../../lib/vm-state.ts";
import { NetworkManager, type NetworkConfig } from "../../lib/network.ts";
import { safeKill, toError } from "../../lib/utils.ts";
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
      error: toError(error).message,
    });
  } catch (err) {
    consola.debug(`Failed to mark VM ${vmId} as error: ${toError(err).message}`);
  }
}

export function cleanupNetwork(networkConfig: NetworkConfig | undefined): void {
  if (!networkConfig) return;
  try {
    NetworkManager.fromConfig(networkConfig).teardown();
  } catch (err) {
    consola.debug(`Network cleanup failed: ${toError(err).message}`);
  }
}

export function cleanupChroot(chrootDir: string | undefined): void {
  if (!chrootDir) return;

  const vmJailerDir = dirname(chrootDir);
  try {
    rmSync(chrootDir, { recursive: true, force: true });
  } catch (err) {
    consola.debug(`Chroot cleanup failed for ${chrootDir}: ${toError(err).message}`);
  }
  try {
    rmSync(vmJailerDir, { recursive: true, force: true });
  } catch (err) {
    consola.debug(`Jailer dir cleanup failed for ${vmJailerDir}: ${toError(err).message}`);
  }
}
