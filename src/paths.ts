import { homedir } from "node:os";
import { join } from "node:path";
import { execSync } from "node:child_process";
import { existsSync } from "node:fs";

export interface VmsanPaths {
  baseDir: string;
  vmsDir: string;
  jailerBaseDir: string;
  binDir: string;
  agentBin: string;
  nftablesBin: string;
  gatewayBin: string;
  dnsproxyBin: string;
  kernelsDir: string;
  rootfsDir: string;
  registryDir: string;
  snapshotsDir: string;
  seccompDir: string;
  seccompFilter: string;
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

/**
 * Resolve a binary path: prefer user-local (~/.vmsan/bin), fallback to /usr/local/bin.
 * The gateway installs privileged binaries to /usr/local/bin for system-wide access.
 */
function resolveBin(base: string, name: string): string {
  const userPath = join(base, "bin", name);
  if (existsSync(userPath)) return userPath;
  const systemPath = join("/usr/local/bin", name);
  if (existsSync(systemPath)) return systemPath;
  return userPath; // return user path even if missing (for error messages)
}

export function vmsanPaths(baseDir?: string): VmsanPaths {
  const base = baseDir ?? resolveBaseDir();
  return {
    baseDir: base,
    vmsDir: join(base, "vms"),
    jailerBaseDir: "/srv/jailer",
    binDir: join(base, "bin"),
    agentBin: resolveBin(base, "vmsan-agent"),
    nftablesBin: resolveBin(base, "vmsan-nftables"),
    gatewayBin: resolveBin(base, "vmsan-gateway"),
    dnsproxyBin: resolveBin(base, "dnsproxy"),
    kernelsDir: join(base, "kernels"),
    rootfsDir: join(base, "rootfs"),
    registryDir: join(base, "registry", "rootfs"),
    snapshotsDir: join(base, "snapshots"),
    seccompDir: join(base, "seccomp"),
    seccompFilter: join(base, "seccomp", "default.json"),
    agentPort: 9119,
  };
}
