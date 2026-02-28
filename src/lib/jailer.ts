import { execFileSync, execSync } from "node:child_process";
import {
  copyFileSync,
  existsSync,
  linkSync,
  mkdirSync,
  readFileSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import {
  generateWelcomeHtml,
  generateWelcomeServer,
  generateWelcomeService,
} from "./welcome-page.ts";
import { generateAgentService, generateAgentEnv } from "./agent-service.ts";

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
  welcomePage?: {
    vmId: string;
    ports: number[];
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

export function detectCgroupVersion(): 1 | 2 {
  try {
    readFileSync("/sys/fs/cgroup/cgroup.controllers", "utf-8");
    return 2;
  } catch {
    return 1;
  }
}

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

export class Jailer {
  readonly paths: JailerPaths;

  constructor(
    private readonly vmId: string,
    jailerBaseDir: string,
  ) {
    const chrootBase = jailerBaseDir;
    const chrootDir = join(chrootBase, "firecracker", vmId);
    const rootDir = join(chrootDir, "root");
    const kernelDir = join(rootDir, "kernel");
    const rootfsDir = join(rootDir, "rootfs");
    const socketDir = join(rootDir, "run");
    const snapshotDir = join(rootDir, "snapshot");

    this.paths = {
      chrootBase,
      chrootDir,
      rootDir,
      kernelDir,
      kernelPath: join(kernelDir, "vmlinux"),
      rootfsDir,
      rootfsPath: join(rootfsDir, "rootfs.ext4"),
      socketDir,
      socketPath: join(socketDir, "firecracker.socket"),
      snapshotDir,
    };
  }

  prepare(config: PrepareChrootConfig): JailerPaths {
    const paths = this.paths;

    // Create directories
    mkdirSync(paths.kernelDir, { recursive: true });
    mkdirSync(paths.rootfsDir, { recursive: true });
    mkdirSync(paths.socketDir, { recursive: true });

    // Hard-link kernel (read-only, shared across VMs)
    if (!existsSync(paths.kernelPath)) {
      linkSync(config.kernelSrc, paths.kernelPath);
    }

    // Copy rootfs (writable per-VM)
    copyFileSync(config.rootfsSrc, paths.rootfsPath);

    // Expand per-VM disk when requested.
    if (typeof config.diskSizeGb === "number" && Number.isFinite(config.diskSizeGb)) {
      const targetBytes = Math.trunc(config.diskSizeGb * 1024 * 1024 * 1024);
      const currentBytes = statSync(paths.rootfsPath).size;
      if (targetBytes > currentBytes) {
        execSync(`truncate -s ${targetBytes} "${paths.rootfsPath}"`, { stdio: "pipe" });
        // e2fsck -fy: force check + auto-fix all issues. resize2fs requires a clean filesystem.
        // Exit code 1 means corrections were made (safe to continue), only fail on code >= 4.
        execSync(`sudo e2fsck -fy "${paths.rootfsPath}"; [ $? -lt 4 ]`, { stdio: "pipe" });
        execSync(`sudo resize2fs "${paths.rootfsPath}"`, { stdio: "pipe" });
        execSync(`sudo tune2fs -m 0 "${paths.rootfsPath}"`, { stdio: "pipe" });
      }
    }

    // Mount rootfs to configure DNS and inject services
    const tmpMount = join(paths.rootDir, "tmp-mount");
    mkdirSync(tmpMount, { recursive: true });
    try {
      execSync(`sudo mount -o loop "${paths.rootfsPath}" "${tmpMount}"`, {
        stdio: "pipe",
      });

      // Configure DNS: symlink /etc/resolv.conf → /proc/net/pnp
      execSync(
        `rm -f "${tmpMount}/etc/resolv.conf" && ln -s /proc/net/pnp "${tmpMount}/etc/resolv.conf"`,
        { stdio: "pipe" },
      );

      // Inject welcome page files for node22-demo runtime.
      if (config.welcomePage) {
        const { vmId: welcomeVmId, ports: welcomePorts } = config.welcomePage;
        const welcomeDir = join(tmpMount, "opt", "vmsan", "welcome");
        mkdirSync(welcomeDir, { recursive: true });
        writeFileSync(
          join(welcomeDir, "index.html"),
          generateWelcomeHtml(welcomeVmId, welcomePorts),
        );
        writeFileSync(join(welcomeDir, "server.js"), generateWelcomeServer(welcomePorts));

        const systemdDir = join(tmpMount, "etc", "systemd", "system");
        mkdirSync(systemdDir, { recursive: true });
        writeFileSync(
          join(systemdDir, "vmsan-welcome.service"),
          generateWelcomeService(welcomePorts),
        );

        // Enable the service at boot via symlink into multi-user.target.wants
        const wantsDir = join(systemdDir, "multi-user.target.wants");
        mkdirSync(wantsDir, { recursive: true });
        execSync(
          `ln -sf /etc/systemd/system/vmsan-welcome.service "${join(wantsDir, "vmsan-welcome.service")}"`,
          { stdio: "pipe" },
        );
      }

      // Inject vmsan-agent binary and systemd service.
      if (config.agent) {
        const agentDst = join(tmpMount, "usr", "local", "bin", "vmsan-agent");
        mkdirSync(join(tmpMount, "usr", "local", "bin"), { recursive: true });
        copyFileSync(config.agent.binaryPath, agentDst);
        execSync(`chmod 755 "${agentDst}"`, { stdio: "pipe" });

        const envDir = join(tmpMount, "etc", "vmsan");
        mkdirSync(envDir, { recursive: true });
        writeFileSync(
          join(envDir, "agent.env"),
          generateAgentEnv(config.agent.token, config.agent.port, config.agent.vmId),
        );

        const systemdDir = join(tmpMount, "etc", "systemd", "system");
        mkdirSync(systemdDir, { recursive: true });
        writeFileSync(join(systemdDir, "vmsan-agent.service"), generateAgentService());

        const wantsDir = join(systemdDir, "multi-user.target.wants");
        mkdirSync(wantsDir, { recursive: true });
        execSync(
          `ln -sf /etc/systemd/system/vmsan-agent.service "${join(wantsDir, "vmsan-agent.service")}"`,
          { stdio: "pipe" },
        );
      }

      execSync(`sudo umount "${tmpMount}"`, { stdio: "pipe" });
    } catch {
      // Mount point may already be unmounted or directory removed
      try {
        execSync(`sudo umount "${tmpMount}" 2>/dev/null`, { stdio: "pipe" });
      } catch {
        // Mount point may already be unmounted
      }
    }
    try {
      execSync(`rm -rf "${tmpMount}"`, { stdio: "pipe" });
    } catch {
      // Temp mount directory may already be removed
    }

    // If restoring from snapshot, copy snapshot files into chroot
    if (config.snapshot) {
      mkdirSync(paths.snapshotDir, { recursive: true });
      copyFileSync(config.snapshot.snapshotFile, join(paths.snapshotDir, "snapshot_file"));
      copyFileSync(config.snapshot.memFile, join(paths.snapshotDir, "mem_file"));
    }

    return paths;
  }

  spawn(config: SpawnJailerConfig): void {
    const uid = config.uid ?? 0;
    const gid = config.gid ?? 0;

    const args = [
      config.jailerBin,
      "--exec-file",
      config.firecrackerBin,
      "--id",
      this.vmId,
      "--uid",
      String(uid),
      "--gid",
      String(gid),
      "--chroot-base-dir",
      config.chrootBase,
      "--daemonize",
    ];

    if (config.newPidNs !== false) {
      args.push("--new-pid-ns");
    }

    if (config.netns) {
      args.push("--netns", `/var/run/netns/${config.netns}`);
    }

    if (config.cgroup) {
      const cgroupVer = detectCgroupVersion();
      if (cgroupVer === 2) {
        args.push("--cgroup-version", "2");
        args.push("--cgroup", `cpu.max=${config.cgroup.cpuQuotaUs} ${config.cgroup.cpuPeriodUs}`);
        args.push("--cgroup", `memory.max=${config.cgroup.memoryBytes}`);
      } else {
        args.push("--cgroup", `cpu.cfs_quota_us=${config.cgroup.cpuQuotaUs}`);
        args.push("--cgroup", `cpu.cfs_period_us=${config.cgroup.cpuPeriodUs}`);
        args.push("--cgroup", `memory.limit_in_bytes=${config.cgroup.memoryBytes}`);
      }
    }

    args.push("--", "--api-sock", "run/firecracker.socket");

    if (config.seccompFilter && existsSync(config.seccompFilter)) {
      args.push("--seccomp-filter", config.seccompFilter);
    } else {
      args.push("--no-seccomp");
    }

    execFileSync("sudo", args, { stdio: "pipe" });
  }
}
