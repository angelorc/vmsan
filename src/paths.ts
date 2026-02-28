import { homedir } from "node:os";
import { join } from "node:path";
import { execSync } from "node:child_process";

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

/**
 * Resolve the real user's home directory, even under sudo.
 * Priority: $VMSAN_DIR > SUDO_USER home > current HOME.
 */
function resolveBaseDir(): string {
  if (process.env.VMSAN_DIR) return process.env.VMSAN_DIR;

  const sudoUser = process.env.SUDO_USER;
  if (sudoUser) {
    try {
      const home = execSync(`getent passwd ${sudoUser}`, { stdio: "pipe" })
        .toString()
        .trim()
        .split(":")[5];
      if (home) return join(home, ".vmsan");
    } catch {
      // fallback below
    }
  }

  return join(homedir(), ".vmsan");
}

export function vmsanPaths(baseDir?: string): VmsanPaths {
  const base = baseDir ?? resolveBaseDir();
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
