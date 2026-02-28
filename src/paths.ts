import { homedir } from "node:os";
import { join } from "node:path";

export interface VmsanPaths {
  baseDir: string;
  vmsDir: string;
  jailerBaseDir: string;
  binDir: string;
  agentBin: string;
  kernelsDir: string;
  rootfsDir: string;
  registryDir: string;
  snapshotsDir: string;
  agentPort: number;
}

export function vmsanPaths(baseDir?: string): VmsanPaths {
  const base = baseDir ?? join(homedir(), ".vmsan");
  return {
    baseDir: base,
    vmsDir: join(base, "vms"),
    jailerBaseDir: join(base, "jailer"),
    binDir: join(base, "bin"),
    agentBin: join(base, "bin", "vmsan-agent"),
    kernelsDir: join(base, "kernels"),
    rootfsDir: join(base, "rootfs"),
    registryDir: join(base, "registry", "rootfs"),
    snapshotsDir: join(base, "snapshots"),
    agentPort: 9119,
  };
}
