export interface JailerPaths {
  chrootBase: string;
  chrootDir: string;
  rootDir: string;
  kernelDir: string;
  kernelPath: string;
  rootfsDir: string;
  rootfsPath: string;
  socketDir: string;
  socketPath: string;
  snapshotDir: string;
}

export interface PrepareChrootConfig {
  kernelSrc: string;
  rootfsSrc: string;
  diskSizeGb?: number;
  snapshot?: {
    snapshotFile: string;
    memFile: string;
  };
  agent?: {
    binaryPath: string;
    token: string;
    port: number;
    vmId: string;
  };
}

export interface CgroupConfig {
  cpuQuotaUs: number;
  cpuPeriodUs: number;
  memoryBytes: number;
}

/**
 * Extra memory (in MiB) added to the cgroup limit beyond guest memory.
 * Covers Firecracker VMM process overhead, page tables, and kernel slab.
 * Without this, the OOM killer can terminate the VM under memory pressure.
 */
export const CGROUP_VMM_OVERHEAD_MIB = 64;

export interface SpawnJailerConfig {
  firecrackerBin: string;
  jailerBin: string;
  chrootBase: string;
  uid?: number;
  gid?: number;
  seccompFilter?: string;
  newPidNs?: boolean;
  cgroup?: CgroupConfig;
  netns?: string;
}
