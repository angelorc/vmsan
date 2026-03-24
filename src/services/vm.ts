import { join } from "node:path";
import { existsSync } from "node:fs";
import type { VmsanContext } from "../context.ts";
import type { VmState, VmStateStore } from "../lib/vm-state.ts";
import {
  validateEnvironment,
  findKernel,
  findBaseRootfs,
  findRuntimeRootfs,
} from "../commands/create/environment.ts";
import { buildInitialVmState } from "../commands/create/state.ts";
import { resolveImageRootfs } from "../commands/create/image-rootfs.ts";
import {
  type ImageReference,
  validatePublishedPortsAvailable,
} from "../commands/create/validation.ts";
import type { NetworkPolicy, Runtime } from "../commands/create/types.ts";
import {
  VmsanError,
  vmNotFoundError,
  vmNotStoppedError,
  vmNotRunningError,
  mutuallyExclusiveFlagsError,
} from "../errors/index.ts";
import { generateVmId, toError } from "../lib/utils.ts";
import { spawnTimeoutKiller } from "../lib/timeout-killer.ts";
import { waitForAgent } from "../lib/vm-context.ts";
import { AgentClient } from "./agent.ts";
import { SnapshotService } from "./snapshot.ts";
import { GatewayClient } from "../lib/gateway-client.ts";
import { slotFromVmHostIp } from "../lib/network-address.ts";
import { FileLock } from "../lib/file-lock.ts";

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
  allowIcmp?: boolean;
  disableNetns?: boolean;
  disableSeccomp?: boolean;
  disablePidNs?: boolean;
  disableCgroup?: boolean;
  timeoutMs?: number;
  snapshotId?: string;
  skipDnat?: boolean;
  connectTo?: string[];
  service?: string;
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

    try {
      // Pre-flight
      validateEnvironment(paths.baseDir);

      if (opts.fromImage && opts.rootfsPath) {
        throw mutuallyExclusiveFlagsError("--from-image", "--rootfs");
      }

      // Hook: beforeCreate (plugins may set opts.skipDnat)
      await hooks.callHook("vm:beforeCreate", { vmId, options: opts });

      // Port conflict check (only when DNAT is active)
      if (!opts.skipDnat) {
        validatePublishedPortsAvailable(opts.ports ?? [], paths);
      }

      // Resolve kernel (keep local resolution, pass absolute path to gateway)
      const kernelPath = opts.kernelPath ?? findKernel(paths.baseDir);
      logger.debug(`Kernel resolved: ${kernelPath}`);

      // Resolve rootfs (keep local resolution)
      let rootfsPath: string;
      if (opts.fromImage) {
        rootfsPath = resolveImageRootfs(opts.fromImage, paths.registryDir, true);
      } else if ((opts.runtime ?? "base") !== "base" && !opts.rootfsPath) {
        rootfsPath = findRuntimeRootfs(opts.runtime as Exclude<Runtime, "base">, paths.baseDir);
      } else {
        rootfsPath = opts.rootfsPath ?? findBaseRootfs(paths.baseDir);
      }
      logger.debug(`Rootfs resolved: ${rootfsPath}`);

      // Reuse agent token from snapshot metadata when restoring;
      // the agent inside the guest already has this token baked in.
      let agentToken: string | null = null;
      if (opts.snapshotId) {
        const meta = SnapshotService.loadMetadata(paths.snapshotsDir, opts.snapshotId);
        agentToken = meta?.agentToken ?? null;
      }

      log.start(`Creating VM ${vmId}...`);

      // Delegate to gateway (single RPC replaces ~15 privileged calls)
      // Seccomp: Firecracker v1.5+ uses its own built-in filter by default.
      // Only pass seccompFilter if the user explicitly provides a custom one.
      const gateway = new GatewayClient();
      const result = await gateway.vmCreate({
        vmId,
        vcpus: opts.vcpus,
        memMib: opts.memMib,
        runtime: opts.runtime,
        diskSizeGb: opts.diskSizeGb,
        networkPolicy: opts.networkPolicy,
        domains: opts.domains,
        allowedCidrs: opts.allowedCidrs,
        deniedCidrs: opts.deniedCidrs,
        ports: opts.ports,
        bandwidthMbit: opts.bandwidthMbit,
        allowIcmp: opts.allowIcmp,
        project: opts.project,
        service: opts.service,
        connectTo: opts.connectTo,
        skipDnat: opts.skipDnat,
        kernelPath,
        rootfsPath,
        snapshotId: opts.snapshotId,
        agentBinary: existsSync(paths.agentBin) ? paths.agentBin : undefined,
        agentToken: agentToken ?? undefined,
        disableSeccomp: opts.disableSeccomp,
        disablePidNs: opts.disablePidNs,
        disableCgroup: opts.disableCgroup,
        jailerBaseDir: paths.jailerBaseDir,
      });

      if (!result.ok || !result.vm) {
        throw new Error(result.error ?? "Gateway vm.create failed");
      }

      const gw = result.vm;

      // Build state from gateway response
      const state = buildInitialVmState({
        vmId,
        project: opts.project || "",
        runtime: opts.runtime ?? "base",
        diskSizeGb: opts.diskSizeGb ?? 10,
        kernelPath,
        rootfsPath,
        vcpus: opts.vcpus ?? 1,
        memMib: opts.memMib ?? 128,
        networkPolicy: opts.networkPolicy ?? "allow-all",
        domains: opts.domains ?? [],
        allowedCidrs: opts.allowedCidrs ?? [],
        deniedCidrs: opts.deniedCidrs ?? [],
        ports: opts.ports ?? [],
        tapDevice: gw.tapDevice,
        hostIp: gw.hostIp,
        guestIp: gw.guestIp,
        subnetMask: gw.subnetMask,
        macAddress: gw.macAddress,
        snapshotId: opts.snapshotId ?? null,
        timeoutMs: opts.timeoutMs ?? null,
        agentToken: gw.agentToken ?? null,
        agentPort: paths.agentPort,
        bandwidthMbit: opts.bandwidthMbit,
        netnsName: gw.netnsName || undefined,
        skipDnat: opts.skipDnat,
        allowIcmp: opts.allowIcmp,
        disableSeccomp: opts.disableSeccomp,
        disablePidNs: opts.disablePidNs,
        disableCgroup: opts.disableCgroup,
        connectTo: opts.connectTo,
        service: opts.service,
      });

      // Update state with gateway response data
      state.chrootDir = gw.chrootDir;
      state.apiSocket = gw.socketPath;
      state.status = "running";
      state.pid = gw.pid;
      if (gw.meshIp) {
        state.network.meshIp = gw.meshIp;
      }
      this.store.save(state);

      log.success(`VM ${vmId} is running (PID: ${gw.pid})`);

      // Timeout killer
      if (opts.timeoutMs && gw.pid) {
        spawnTimeoutKiller({
          vmId,
          pid: gw.pid,
          timeoutMs: opts.timeoutMs,
          stateFile: join(paths.vmsDir, `${vmId}.json`),
        });
      }

      // Forward published ports to localhost inside the VM
      if (gw.agentToken && opts.ports?.length) {
        await this.setupLocalhostPortForwarding(
          gw.guestIp,
          paths.agentPort,
          gw.agentToken,
          opts.ports,
        );
      }

      const finalState = this.store.load(vmId)!;

      // Hook: afterCreate
      await hooks.callHook("vm:afterCreate", finalState);

      // Re-read state: hooks (e.g. Cloudflare plugin) may have updated it
      const updatedState = this.store.load(vmId) ?? finalState;

      return { vmId, pid: gw.pid, state: updatedState };
    } catch (error) {
      // Error hooks
      if (vmId) {
        await hooks.callHook("vm:error", {
          vmId,
          error: toError(error),
          phase: "create",
        });
      }

      // Gateway handles rollback on its side; just mark state as error
      this.markAsError(vmId, error);

      throw error;
    }
  }

  // -----------------------------------------------------------------------
  // start
  // -----------------------------------------------------------------------

  async start(vmId: string): Promise<StartVmResult> {
    const { logger, paths, hooks } = this;
    const log = logger.withTag(vmId);

    try {
      // 1. Validate state
      const state = this.store.load(vmId);
      if (!state) {
        throw vmNotFoundError(vmId);
      }
      if (state.status !== "stopped") {
        throw vmNotStoppedError(vmId, state.status);
      }

      validateEnvironment(paths.baseDir);

      // Hook: beforeStart
      await hooks.callHook("vm:beforeStart", { vmId, state });

      log.start(`Starting VM ${vmId}...`);

      // Derive slot from stored network config
      const slot = slotFromVmHostIp(state.network.hostIp);

      // Delegate to gateway
      // Seccomp: Firecracker v1.5+ uses its own built-in filter by default.
      const gateway = new GatewayClient();
      const result = await gateway.vmRestart({
        vmId,
        slot,
        chrootDir: state.chrootDir || undefined,
        socketPath: state.apiSocket || undefined,
        networkPolicy: state.network.networkPolicy,
        domains: state.network.allowedDomains,
        allowedCidrs: state.network.allowedCidrs,
        deniedCidrs: state.network.deniedCidrs,
        ports: state.network.publishedPorts,
        bandwidthMbit: state.network.bandwidthMbit,
        allowIcmp: state.network.allowIcmp,
        skipDnat: state.network.skipDnat,
        project: state.project || undefined,
        service: state.network.service,
        connectTo: state.network.connectTo,
        disableSeccomp: state.disableSeccomp,
        disablePidNs: state.disablePidNs,
        disableCgroup: state.disableCgroup,
        vcpus: state.vcpuCount,
        memMib: state.memSizeMib,
        kernelPath: state.kernel,
        rootfsPath: state.rootfs,
        agentBinary: existsSync(paths.agentBin) ? paths.agentBin : undefined,
        agentToken: state.agentToken || undefined,
        netnsName: state.network.netnsName,
        jailerBaseDir: paths.jailerBaseDir,
      });

      if (!result.ok || !result.vm) {
        throw new Error(result.error ?? "Gateway vm.restart failed");
      }

      const gw = result.vm;

      // Update state with new PID and running status
      const updates: Partial<VmState> = {
        status: "running",
        pid: gw.pid,
      };
      if (gw.socketPath) {
        updates.apiSocket = gw.socketPath;
      }
      if (gw.chrootDir) {
        updates.chrootDir = gw.chrootDir;
      }
      if (gw.meshIp) {
        updates.network = {
          ...state.network,
          meshIp: gw.meshIp,
        };
      }
      this.store.update(vmId, updates);

      log.success(`VM ${vmId} is running (PID: ${gw.pid})`);

      // Re-apply localhost port forwarding after restart
      if (state.agentToken && state.network.publishedPorts?.length) {
        await this.setupLocalhostPortForwarding(
          state.network.guestIp,
          state.agentPort,
          state.agentToken,
          state.network.publishedPorts,
        );
      }

      const finalState = this.store.load(vmId)!;

      // Hook: afterStart
      await hooks.callHook("vm:afterStart", finalState);

      return { vmId, pid: gw.pid, state: finalState, success: true };
    } catch (error) {
      // Error hooks
      await hooks.callHook("vm:error", {
        vmId,
        error: toError(error),
        phase: "start",
      });

      this.markAsError(vmId, error);

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

      // Derive slot from stored network config
      const slot = slotFromVmHostIp(state.network.hostIp);

      // Delegate to gateway
      const gateway = new GatewayClient();
      const result = await gateway.vmFullStop({
        vmId,
        slot,
        pid: state.pid ?? undefined,
        netnsName: state.network.netnsName,
        socketPath: state.apiSocket || undefined,
        jailerBaseDir: this.paths.jailerBaseDir,
      });

      if (!result.ok) {
        this.logger.debug(`Gateway vm.fullStop warning: ${result.error ?? "unknown"}`);
      }

      // Update state
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
      return { vmId, success: false, error: toError(err) };
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
    allowIcmp?: boolean,
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
        // Resolve allowIcmp: explicit flag wins, otherwise preserve existing
        const effectiveAllowIcmp = allowIcmp ?? state.network.allowIcmp ?? false;

        // Derive slot from stored network config
        const slot = slotFromVmHostIp(state.network.hostIp);

        // Delegate to gateway
        const gateway = new GatewayClient();
        const result = await gateway.vmFullUpdatePolicy({
          vmId,
          policy,
          slot,
          domains,
          allowedCidrs,
          deniedCidrs,
          ports: state.network.publishedPorts,
          allowIcmp: effectiveAllowIcmp,
          skipDnat: state.network.skipDnat,
          netnsName: state.network.netnsName,
        });

        if (!result.ok) {
          throw new Error(result.error ?? "Gateway vm.fullUpdatePolicy failed");
        }

        this.store.update(vmId, {
          network: {
            ...state.network,
            networkPolicy: policy,
            allowedDomains: domains,
            allowedCidrs,
            deniedCidrs,
            allowIcmp: effectiveAllowIcmp,
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
          error: toError(err),
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

      // 2. Delegate chroot cleanup to gateway
      const gateway = new GatewayClient();
      const result = await gateway.vmDelete({ vmId, force, jailerBaseDir: this.paths.jailerBaseDir });
      if (!result.ok) {
        this.logger.debug(`Gateway vm.delete warning: ${result.error ?? "unknown"}`);
      }

      // 3. Delete state file -- VM disappears from list
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
      return { vmId, success: false, error: toError(err) };
    }
  }

  // -----------------------------------------------------------------------
  // Private helpers
  // -----------------------------------------------------------------------

  /**
   * Set up iptables DNAT rules inside the VM so that traffic arriving on
   * published ports is forwarded to 127.0.0.1. Many services bind to
   * localhost only; this lets the Cloudflare tunnel (which connects to the
   * guest IP) reach them.
   */
  private async setupLocalhostPortForwarding(
    guestIp: string,
    agentPort: number,
    agentToken: string,
    ports: number[],
  ): Promise<void> {
    try {
      await waitForAgent(guestIp, agentPort);
      const agent = new AgentClient(`http://${guestIp}:${agentPort}`, agentToken);
      const iptablesRules = ports
        .map(
          (p) =>
            `sudo iptables-legacy -t nat -A PREROUTING -i eth0 -p tcp --dport ${p} -j DNAT --to-destination 127.0.0.1:${p}`,
        )
        .join(" && ");
      await agent.runCommand({
        cmd: "/bin/bash",
        args: ["-c", `sudo sysctl -w net.ipv4.conf.all.route_localnet=1 && ${iptablesRules}`],
      });
      this.logger.debug(`Localhost port forwarding set up for ports: ${ports.join(", ")}`);
    } catch (err) {
      this.logger.warn(`Failed to set up localhost port forwarding: ${toError(err).message}`);
    }
  }

  private markAsError(vmId: string, error: unknown): void {
    try {
      this.store.update(vmId, {
        status: "error",
        error: toError(error).message,
      });
    } catch (err) {
      this.logger.warn(`Failed to mark VM ${vmId} as error: ${toError(err).message}`);
    }
  }
}
