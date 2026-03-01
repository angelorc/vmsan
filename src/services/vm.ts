import { dirname, join } from "node:path";
import { existsSync, rmSync } from "node:fs";
import { randomBytes } from "node:crypto";
import { spawn } from "node:child_process";
import type { VmsanContext } from "../context.ts";
import type { VmState, VmStateStore } from "../lib/vm-state.ts";
import { NetworkManager, type NetworkConfig } from "../lib/network.ts";
import { FileLock } from "../lib/file-lock.ts";
import { FirecrackerClient } from "./firecracker.ts";
import { Jailer, type CgroupConfig, CGROUP_VMM_OVERHEAD_MIB } from "../lib/jailer.ts";
import {
  getVmJailerPid,
  getVmPid,
  validateEnvironment,
  findKernel,
  findRootfs,
  waitForSocket,
} from "../commands/create/environment.ts";
import { killOrphanVmProcess, cleanupNetwork, cleanupChroot } from "../commands/create/cleanup.ts";
import { buildInitialVmState } from "../commands/create/state.ts";
import { resolveImageRootfs } from "../commands/create/image-rootfs.ts";
import type { ImageReference } from "../commands/create/validation.ts";
import { ensureSeccompFilter } from "../lib/seccomp.ts";
import type { NetworkPolicy, Runtime } from "../commands/create/types.ts";
import {
  VmsanError,
  vmNotFoundError,
  vmNotStoppedError,
  vmNotRunningError,
  chrootNotFoundError,
  mutuallyExclusiveFlagsError,
} from "../errors/index.ts";
import { generateVmId, safeKill, toError } from "../lib/utils.ts";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface CreateVmOptions {
  vcpus?: number;
  memMib?: number;
  diskSizeGb?: number;
  kernelPath?: string;
  rootfsPath?: string;
  fromImage?: ImageReference;
  project?: string;
  runtime?: Runtime;
  networkPolicy?: NetworkPolicy;
  domains?: string[];
  allowedCidrs?: string[];
  deniedCidrs?: string[];
  ports?: number[];
  bandwidthMbit?: number;
  disableNetns?: boolean;
  disableSeccomp?: boolean;
  disablePidNs?: boolean;
  disableCgroup?: boolean;
  timeoutMs?: number;
  snapshotId?: string;
}

export interface CreateVmResult {
  vmId: string;
  pid: number | null;
  state: VmState;
}

export interface StartVmResult {
  vmId: string;
  pid: number | null;
  state: VmState | null;
  success: boolean;
  error?: VmsanError;
}

export interface StopResult {
  vmId: string;
  success: boolean;
  error?: VmsanError;
  alreadyStopped?: boolean;
}

export interface UpdatePolicyResult {
  vmId: string;
  success: boolean;
  previousPolicy: NetworkPolicy;
  newPolicy: NetworkPolicy;
  error?: VmsanError;
}

// ---------------------------------------------------------------------------
// VMService
// ---------------------------------------------------------------------------

export class VMService {
  readonly paths: VmsanContext["paths"];
  readonly store: VmStateStore;
  readonly hooks: VmsanContext["hooks"];
  readonly logger: VmsanContext["logger"];

  constructor(ctx: VmsanContext) {
    this.paths = ctx.paths;
    this.store = ctx.store;
    this.hooks = ctx.hooks;
    this.logger = ctx.logger;
  }

  // -----------------------------------------------------------------------
  // Read operations
  // -----------------------------------------------------------------------

  list(): VmState[] {
    return this.store
      .list()
      .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());
  }

  get(vmId: string): VmState | null {
    return this.store.load(vmId);
  }

  // -----------------------------------------------------------------------
  // create
  // -----------------------------------------------------------------------

  async create(opts: CreateVmOptions): Promise<CreateVmResult> {
    const { logger, paths, hooks } = this;

    const vmId = generateVmId();
    const log = logger.withTag(vmId);

    let networkConfig: NetworkConfig | undefined;
    let chrootDir: string | undefined;

    try {
      // Pre-flight
      validateEnvironment(paths.baseDir);

      if (opts.fromImage && opts.rootfsPath) {
        throw mutuallyExclusiveFlagsError("--from-image", "--rootfs");
      }

      const vcpus = opts.vcpus ?? 1;
      const memMib = opts.memMib ?? 128;
      const diskSizeGb = opts.diskSizeGb ?? 10;
      const runtime: Runtime = opts.runtime ?? "base";
      const networkPolicy: NetworkPolicy = opts.networkPolicy ?? "allow-all";
      const domains = opts.domains ?? [];
      const allowedCidrs = opts.allowedCidrs ?? [];
      const deniedCidrs = opts.deniedCidrs ?? [];
      const ports = opts.ports ?? [];
      const bandwidthMbit = opts.bandwidthMbit;
      const snapshotId = opts.snapshotId ?? null;
      const timeoutMs = opts.timeoutMs ?? null;

      // Hook: beforeCreate
      await hooks.callHook("vm:beforeCreate", { vmId, options: opts });

      // Resolve kernel
      const kernelPath = opts.kernelPath ?? findKernel(paths.baseDir);
      logger.debug(`Kernel resolved: ${kernelPath}`);

      // Resolve rootfs
      let rootfsPath: string;
      if (opts.fromImage) {
        rootfsPath = resolveImageRootfs(opts.fromImage, paths.registryDir);
      } else {
        rootfsPath = opts.rootfsPath ?? findRootfs(paths.baseDir);
      }
      logger.debug(`Rootfs resolved: ${rootfsPath}`);

      const netnsName = opts.disableNetns ? undefined : `vmsan-${vmId}`;
      const agentToken = existsSync(paths.agentBin) ? randomBytes(32).toString("hex") : null;

      log.start(`Creating VM ${vmId}...`);

      // Allocate network slot + save initial state (under lock)
      const slotLock = new FileLock(join(paths.vmsDir, ".slot-lock"), "slot-alloc");
      const { net } = slotLock.run(() => {
        const slot = this.store.allocateNetworkSlot();
        logger.debug(`Network slot allocated: ${slot}`);
        const net = new NetworkManager(
          slot,
          networkPolicy,
          domains,
          allowedCidrs,
          deniedCidrs,
          ports,
          bandwidthMbit,
          netnsName,
        );
        networkConfig = net.config;

        const state = buildInitialVmState({
          vmId,
          project: opts.project || "",
          runtime,
          diskSizeGb,
          kernelPath,
          rootfsPath,
          vcpus,
          memMib,
          networkPolicy,
          domains,
          allowedCidrs,
          deniedCidrs,
          ports,
          tapDevice: net.config.tapDevice,
          hostIp: net.config.hostIp,
          guestIp: net.config.guestIp,
          subnetMask: net.config.subnetMask,
          macAddress: net.config.macAddress,
          snapshotId,
          timeoutMs,
          agentToken,
          agentPort: paths.agentPort,
          bandwidthMbit,
          netnsName,
        });
        this.store.save(state);

        return { net };
      });
      const netCfg = networkConfig!;

      // Setup networking
      log.start("Setting up networking...");
      await net.setup();
      log.success(
        `Network: TAP ${netCfg.tapDevice}, Host ${netCfg.hostIp}, Guest ${netCfg.guestIp}`,
      );

      // Hook: network:afterSetup
      await hooks.callHook("network:afterSetup", {
        vmId,
        slot: netCfg.slot,
        networkConfig: netCfg,
        domains,
        networkPolicy,
      });

      // Prepare chroot
      log.start("Preparing chroot...");
      const snapshotConfig = snapshotId
        ? {
            snapshotFile: join(paths.snapshotsDir, snapshotId, "snapshot_file"),
            memFile: join(paths.snapshotsDir, snapshotId, "mem_file"),
          }
        : undefined;

      const jailer = new Jailer(vmId, paths.jailerBaseDir);
      const welcomePage =
        runtime === "node22-demo" && ports.length > 0 ? { vmId, ports } : undefined;

      const agentConfig = agentToken
        ? {
            binaryPath: paths.agentBin,
            token: agentToken,
            port: paths.agentPort,
            vmId,
          }
        : undefined;

      const jailerPaths = jailer.prepare({
        kernelSrc: kernelPath,
        rootfsSrc: rootfsPath,
        diskSizeGb,
        snapshot: snapshotConfig,
        welcomePage,
        agent: agentConfig,
      });
      chrootDir = jailerPaths.chrootDir;

      this.store.update(vmId, {
        chrootDir: jailerPaths.chrootDir,
        apiSocket: jailerPaths.socketPath,
      });
      logger.debug(`Jailer chroot: ${jailerPaths.chrootDir}`);
      logger.debug(`API socket path: ${jailerPaths.socketPath}`);

      // Spawn Firecracker
      log.start("Spawning Firecracker via jailer...");
      const firecrackerBin = join(paths.binDir, "firecracker");
      const jailerBin = join(paths.binDir, "jailer");

      const seccompFilter = opts.disableSeccomp ? undefined : ensureSeccompFilter(paths);
      if (seccompFilter) {
        logger.debug(`Seccomp filter: ${seccompFilter}`);
      }

      const cgroup = opts.disableCgroup ? undefined : this.buildCgroupConfig(vcpus, memMib);

      jailer.spawn({
        firecrackerBin,
        jailerBin,
        chrootBase: jailerPaths.chrootBase,
        seccompFilter: seccompFilter ?? undefined,
        newPidNs: !opts.disablePidNs,
        cgroup,
        netns: netnsName,
      });

      log.start("Waiting for API socket...");
      await waitForSocket(jailerPaths.socketPath, 5000);
      log.success("API socket ready");

      // Boot VM
      if (snapshotId) {
        log.start("Restoring from snapshot...");
        const vm = new FirecrackerClient(jailerPaths.socketPath);
        await vm.loadSnapshot("snapshot/snapshot_file", "snapshot/mem_file");
        await vm.resume();
        log.success("Snapshot restored and VM resumed");
      } else {
        await this.bootVm(jailerPaths.socketPath, netCfg, vcpus, memMib);
        log.start("Starting VM...");
        const vm = new FirecrackerClient(jailerPaths.socketPath);
        await vm.start();
      }

      const pid = getVmPid(vmId);
      logger.debug(`Firecracker PID: ${pid ?? "unknown"}`);
      this.store.update(vmId, { status: "running", pid });
      log.success(`VM ${vmId} is running (PID: ${pid || "unknown"})`);

      // Timeout killer
      if (timeoutMs && pid) {
        const stateFile = join(paths.vmsDir, `${vmId}.json`);
        const killer = spawn(
          "bash",
          [
            "-c",
            [
              `sleep ${Math.ceil(timeoutMs / 1000)}`,
              `STATE=$(cat "${stateFile}" 2>/dev/null) || exit 0`,
              `echo "$STATE" | grep -q '"status":"running"' || exit 0`,
              `echo "$STATE" | grep -q '"pid":${pid}' || exit 0`,
              `[ -d /proc/${pid} ] || exit 0`,
              `grep -q "${vmId}" /proc/${pid}/cmdline 2>/dev/null || exit 0`,
              `kill ${pid} 2>/dev/null`,
            ].join(" && "),
          ],
          { detached: true, stdio: "ignore" },
        );
        killer.unref();
      }

      const finalState = this.store.load(vmId)!;

      // Hook: afterCreate
      await hooks.callHook("vm:afterCreate", finalState);

      return { vmId, pid, state: finalState };
    } catch (error) {
      // Error hooks
      if (vmId) {
        await hooks.callHook("vm:error", {
          vmId,
          error: toError(error),
          phase: "create",
        });
      }

      // Cleanup
      killOrphanVmProcess(vmId);
      this.markAsError(vmId, error);
      cleanupNetwork(networkConfig);
      cleanupChroot(chrootDir);

      throw error;
    }
  }

  // -----------------------------------------------------------------------
  // start
  // -----------------------------------------------------------------------

  async start(vmId: string): Promise<StartVmResult> {
    const { logger, paths, hooks } = this;
    const log = logger.withTag(vmId);

    let networkConfig: NetworkConfig | undefined;

    try {
      // 1. Validate state
      const state = this.store.load(vmId);
      if (!state) {
        throw vmNotFoundError(vmId);
      }
      if (state.status !== "stopped") {
        throw vmNotStoppedError(vmId, state.status);
      }
      if (!state.chrootDir || !existsSync(state.chrootDir)) {
        throw chrootNotFoundError(vmId);
      }

      validateEnvironment(paths.baseDir);

      // Hook: beforeStart
      await hooks.callHook("vm:beforeStart", { vmId, state });

      log.start(`Starting VM ${vmId}...`);

      // 2. Reconstruct network config and re-setup networking
      const mgr = NetworkManager.fromVmNetwork(state.network);
      networkConfig = mgr.config;
      logger.debug(
        `Reconstructed network config: slot=${networkConfig.slot}, tap=${networkConfig.tapDevice}, host=${networkConfig.hostIp}, guest=${networkConfig.guestIp}`,
      );

      log.start("Setting up networking...");
      await mgr.setup();
      log.success(
        `Network: TAP ${networkConfig.tapDevice}, Host ${networkConfig.hostIp}, Guest ${networkConfig.guestIp}`,
      );

      // Hook: network:afterSetup
      await hooks.callHook("network:afterSetup", {
        vmId,
        slot: networkConfig.slot,
        networkConfig,
        domains: state.network.allowedDomains,
        networkPolicy: state.network.networkPolicy as NetworkPolicy,
      });

      // 3. Clean stale files from previous run
      const vmRootCandidates = Array.from(
        new Set([
          join(state.chrootDir, "root"),
          state.chrootDir,
          dirname(dirname(state.apiSocket)),
        ]),
      );
      const removeStaleFirecrackerFiles = (): void => {
        for (const rootDir of vmRootCandidates) {
          rmSync(join(rootDir, "firecracker"), { force: true });
          rmSync(join(rootDir, "firecracker.pid"), { force: true });
        }
      };

      removeStaleFirecrackerFiles();

      const socketPath = state.apiSocket;
      rmSync(socketPath, { force: true });

      const removeStaleDevTrees = (): void => {
        for (const rootDir of vmRootCandidates) {
          rmSync(join(rootDir, "dev"), { recursive: true, force: true });
        }
      };
      const removeStaleDeviceNodes = (): void => {
        const staleNodes = ["dev/net/tun", "dev/kvm", "dev/userfaultfd", "dev/urandom"];
        for (const rootDir of vmRootCandidates) {
          for (const rel of staleNodes) {
            rmSync(join(rootDir, rel), { recursive: true, force: true });
          }
        }
      };

      // 4. Spawn Firecracker via Jailer (reuse existing chroot)
      const firecrackerBin = join(paths.binDir, "firecracker");
      const jailerBin = join(paths.binDir, "jailer");

      const jailer = new Jailer(vmId, paths.jailerBaseDir);
      let socketReady = false;
      const startTag = `[start:${vmId}]`;
      const logAttemptError = (attempt: string, error: unknown): void => {
        logger.error(`${startTag} ${attempt} failed: ${toError(error).message}`);
      };
      logger.debug(`Stale file cleanup: checked ${vmRootCandidates.length} root candidates`);

      const logDiagnostics = (): void => {
        const socketExists = existsSync(socketPath);
        const devState = vmRootCandidates
          .map((rootDir) => `${join(rootDir, "dev")}=${existsSync(join(rootDir, "dev"))}`)
          .join(", ");
        const firecrackerPid = getVmPid(vmId);
        const jailerPid = getVmJailerPid(vmId);
        log.error(
          `${startTag} diagnostics: socketExists=${socketExists}; firecrackerPid=${firecrackerPid ?? "none"}; jailerPid=${jailerPid ?? "none"}; devDirs=[${devState}]`,
        );
      };

      const isRecoverableStartError = (message: string): boolean => {
        if (message.includes("Timeout waiting for API socket")) return true;
        if (message.includes("mknod inside the jail") && message.includes("File exists")) {
          return true;
        }
        if (message.includes("MknodDev(") && message.includes("os error 17")) {
          return true;
        }
        return false;
      };

      const cgroup = this.buildCgroupConfig(state.vcpuCount, state.memSizeMib);

      const spawnAndWait = async (timeoutMs: number): Promise<void> => {
        log.start("Spawning Firecracker via jailer...");
        logger.debug(
          `Jailer spawn: firecracker=${firecrackerBin}, jailer=${jailerBin}, chrootBase=${jailer.paths.chrootBase}`,
        );
        jailer.spawn({
          firecrackerBin,
          jailerBin,
          chrootBase: jailer.paths.chrootBase,
          newPidNs: true,
          cgroup,
          netns: state.network.netnsName,
        });

        log.start("Waiting for API socket...");
        await waitForSocket(socketPath, timeoutMs);
      };

      try {
        removeStaleDeviceNodes();
        await spawnAndWait(10000);
        socketReady = true;
      } catch (firstStartError) {
        const message = toError(firstStartError).message;
        if (!isRecoverableStartError(message)) {
          logAttemptError("initial attempt", firstStartError);
          logDiagnostics();
          throw firstStartError;
        }

        logAttemptError("initial attempt", firstStartError);
        killOrphanVmProcess(vmId);
        rmSync(socketPath, { force: true });
        removeStaleDeviceNodes();
        removeStaleDevTrees();
        removeStaleFirecrackerFiles();

        try {
          await spawnAndWait(15000);
          socketReady = true;
        } catch (retryError) {
          logAttemptError("retry attempt", retryError);
          logDiagnostics();
          throw new Error(
            `${startTag} retry failed after cleanup. First error: ${message}. Retry error: ${toError(retryError).message}`,
          );
        }
      }

      if (!socketReady) {
        throw new Error(`Timeout waiting for API socket at ${socketPath}`);
      }

      log.success("API socket ready");

      // 5. Boot VM
      await this.bootVm(socketPath, networkConfig, state.vcpuCount, state.memSizeMib);

      log.start("Starting VM...");
      const vm = new FirecrackerClient(socketPath);
      await vm.start();

      // 6. Update state
      const pid = getVmPid(vmId);
      this.store.update(vmId, { status: "running", pid });
      log.success(`VM ${vmId} is running (PID: ${pid || "unknown"})`);

      const finalState = this.store.load(vmId)!;

      // Hook: afterStart
      await hooks.callHook("vm:afterStart", finalState);

      return { vmId, pid, state: finalState, success: true };
    } catch (error) {
      // Error hooks
      await hooks.callHook("vm:error", {
        vmId,
        error: toError(error),
        phase: "start",
      });

      killOrphanVmProcess(vmId);
      this.markAsError(vmId, error);
      cleanupNetwork(networkConfig);

      return {
        vmId,
        pid: null,
        state: null,
        success: false,
        error: error instanceof VmsanError ? error : undefined,
      };
    }
  }

  // -----------------------------------------------------------------------
  // stop
  // -----------------------------------------------------------------------

  async stop(vmId: string): Promise<StopResult> {
    const state = this.store.load(vmId);
    if (!state) {
      return { vmId, success: false, error: vmNotFoundError(vmId) };
    }

    if (state.status === "stopped") {
      return { vmId, success: true, alreadyStopped: true };
    }

    try {
      // Hook: beforeStop
      await this.hooks.callHook("vm:beforeStop", { vmId, state });

      const previousStatus = state.status;

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
        const netCfg = NetworkManager.fromVmNetwork(state.network);
        try {
          netCfg.teardown();
        } catch {
          // Network may be partially torn down from a prior attempt
        }

        // Hook: network:afterTeardown
        await this.hooks.callHook("network:afterTeardown", {
          vmId,
          networkConfig: netCfg.config,
        });
      }

      // 3. Update state
      this.store.update(vmId, { status: "stopped", pid: null });

      // Hook: afterStop
      await this.hooks.callHook("vm:afterStop", { vmId, previousStatus });

      return { vmId, success: true };
    } catch (err) {
      await this.hooks.callHook("vm:error", {
        vmId,
        error: toError(err),
        phase: "stop",
      });
      return { vmId, success: false, error: err instanceof VmsanError ? err : undefined };
    }
  }

  // -----------------------------------------------------------------------
  // updateNetworkPolicy
  // -----------------------------------------------------------------------

  async updateNetworkPolicy(
    vmId: string,
    policy: NetworkPolicy,
    domains: string[],
    allowedCidrs: string[],
    deniedCidrs: string[],
  ): Promise<UpdatePolicyResult> {
    const stateFile = join(this.paths.vmsDir, `${vmId}.json`);
    const fileLock = new FileLock(stateFile, `update-policy-${vmId}`);

    return fileLock.runAsync(async () => {
      const state = this.store.load(vmId);
      if (!state) {
        return {
          vmId,
          success: false,
          previousPolicy: policy,
          newPolicy: policy,
          error: vmNotFoundError(vmId),
        };
      }

      const previousPolicy = state.network.networkPolicy as NetworkPolicy;

      if (state.status !== "running") {
        return {
          vmId,
          success: false,
          previousPolicy,
          newPolicy: policy,
          error: vmNotRunningError(vmId),
        };
      }

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

        // Hook: network:policyChange
        await this.hooks.callHook("network:policyChange", {
          vmId,
          previousPolicy,
          newPolicy: policy,
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

  // -----------------------------------------------------------------------
  // remove
  // -----------------------------------------------------------------------

  async remove(vmId: string, opts?: { force?: boolean }): Promise<StopResult> {
    const state = this.store.load(vmId);
    if (!state) {
      return { vmId, success: false, error: vmNotFoundError(vmId) };
    }

    try {
      const force = opts?.force ?? false;

      // Hook: beforeRemove
      await this.hooks.callHook("vm:beforeRemove", { vmId, state, force });

      // 1. Stop if running (only when --force is used; caller should pre-check)
      if (state.status !== "stopped") {
        if (!force) {
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
      cleanupChroot(state.chrootDir);

      // 3. Delete state file — VM disappears from list
      this.store.delete(vmId);

      // Hook: afterRemove
      await this.hooks.callHook("vm:afterRemove", { vmId });

      return { vmId, success: true };
    } catch (err) {
      await this.hooks.callHook("vm:error", {
        vmId,
        error: toError(err),
        phase: "remove",
      });
      return { vmId, success: false, error: err instanceof VmsanError ? err : undefined };
    }
  }

  // -----------------------------------------------------------------------
  // Private helpers
  // -----------------------------------------------------------------------

  private buildCgroupConfig(vcpus: number, memMib: number): CgroupConfig {
    return {
      cpuQuotaUs: vcpus * 100000,
      cpuPeriodUs: 100000,
      memoryBytes: (memMib + CGROUP_VMM_OVERHEAD_MIB) * 1024 * 1024,
    };
  }

  private async bootVm(
    socketPath: string,
    netCfg: NetworkConfig,
    vcpus: number,
    memMib: number,
  ): Promise<void> {
    const vm = new FirecrackerClient(socketPath);
    const bootArgs = NetworkManager.bootArgs(netCfg.slot);
    this.logger.debug(`Boot args: ${bootArgs}`);
    await vm.boot("kernel/vmlinux", bootArgs);
    await vm.addDrive("rootfs", "rootfs/rootfs.ext4", true, false);
    await vm.configure(vcpus, memMib);
    await vm.addNetwork("eth0", netCfg.tapDevice, netCfg.macAddress);
  }

  private markAsError(vmId: string, error: unknown): void {
    try {
      this.store.update(vmId, {
        status: "error",
        error: toError(error).message,
      });
    } catch {
      // State store may be corrupt during error recovery
    }
  }
}
