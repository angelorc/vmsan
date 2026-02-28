import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { existsSync, rmSync, unlinkSync } from "node:fs";
import { dirname, join } from "node:path";
import { consola } from "consola";
import { vmsanPaths } from "../paths.ts";
import { createCommandLogger, createScopedLogger } from "../lib/logger/index.ts";
import {
  vmNotFoundError,
  vmNotStoppedError,
  chrootNotFoundError,
  handleCommandError,
} from "../errors/index.ts";
import { FileVmStateStore } from "../lib/vm-state.ts";
import { Jailer, type CgroupConfig, CGROUP_VMM_OVERHEAD_MIB } from "../lib/jailer.ts";
import { NetworkManager, type NetworkConfig } from "../lib/network.ts";
import { FirecrackerClient } from "../services/firecracker.ts";
import {
  getVmJailerPid,
  getVmPid,
  validateEnvironment,
  waitForSocket,
} from "./create/environment.ts";
import { cleanupNetwork, killOrphanVmProcess, markVmAsError } from "./create/cleanup.ts";

const startCommand = defineCommand({
  meta: {
    name: "start",
    description: "Start a previously stopped VM",
  },
  args: {
    vmId: {
      type: "positional",
      description: "VM ID to start",
      required: true,
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("start");
    const paths = vmsanPaths();
    const lifecycle = {
      vmId: undefined as string | undefined,
      networkConfig: undefined as NetworkConfig | undefined,
    };

    const store = new FileVmStateStore(paths.vmsDir);

    try {
      const vmId = args.vmId as string;
      lifecycle.vmId = vmId;
      const log = createScopedLogger(vmId);
      const startTag = `[start:${vmId}]`;

      // 1. Validate state
      const state = store.load(vmId);
      if (!state) {
        throw vmNotFoundError(vmId);
      }
      if (state.status !== "stopped") {
        throw vmNotStoppedError(vmId, state.status);
      }
      if (!state.chrootDir || !existsSync(state.chrootDir)) {
        throw chrootNotFoundError(vmId);
      }

      const baseDir = paths.baseDir;
      validateEnvironment(baseDir);

      log.start(`Starting VM ${vmId}...`);

      // 2. Reconstruct network config and re-setup networking
      const mgr = NetworkManager.fromVmNetwork(state.network);
      const networkConfig = mgr.config;
      lifecycle.networkConfig = networkConfig;
      consola.debug(
        `Reconstructed network config: slot=${networkConfig.slot}, tap=${networkConfig.tapDevice}, host=${networkConfig.hostIp}, guest=${networkConfig.guestIp}`,
      );

      log.start("Setting up networking...");
      await mgr.setup();
      log.success(
        `Network: TAP ${networkConfig.tapDevice}, Host ${networkConfig.hostIp}, Guest ${networkConfig.guestIp}`,
      );

      // 3. Clean stale files from previous run
      const vmRootCandidates = Array.from(
        new Set([
          join(state.chrootDir, "root"),
          state.chrootDir,
          dirname(dirname(state.apiSocket)),
        ]),
      );
      for (const rootDir of vmRootCandidates) {
        const staleFirecrackerBin = join(rootDir, "firecracker");
        if (existsSync(staleFirecrackerBin)) {
          unlinkSync(staleFirecrackerBin);
        }
        rmSync(join(rootDir, "firecracker.pid"), { force: true });
      }

      const socketPath = state.apiSocket;
      if (existsSync(socketPath)) {
        unlinkSync(socketPath);
      }

      const removeStaleDevTrees = (): void => {
        for (const rootDir of vmRootCandidates) {
          const devDir = join(rootDir, "dev");
          if (existsSync(devDir)) {
            rmSync(devDir, { recursive: true, force: true });
          }
        }
      };
      const removeStaleDeviceNodes = (): void => {
        const staleNodes = ["dev/net/tun", "dev/kvm", "dev/userfaultfd", "dev/urandom"];
        for (const rootDir of vmRootCandidates) {
          for (const rel of staleNodes) {
            const nodePath = join(rootDir, rel);
            if (existsSync(nodePath)) {
              rmSync(nodePath, { recursive: true, force: true });
            }
          }
        }
      };

      // 4. Spawn Firecracker via Jailer (reuse existing chroot)
      const firecrackerBin = join(baseDir, "bin", "firecracker");
      const jailerBin = join(baseDir, "bin", "jailer");

      const jailer = new Jailer(vmId, paths.jailerBaseDir);
      let socketReady = false;
      const errorMessage = (error: unknown): string =>
        error instanceof Error ? error.message : String(error);
      const logAttemptError = (attempt: string, error: unknown): void => {
        const message = errorMessage(error);
        consola.error(`${startTag} ${attempt} failed: ${message}`);
      };
      consola.debug(`Stale file cleanup: checked ${vmRootCandidates.length} root candidates`);

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

      const cgroup: CgroupConfig = {
        cpuQuotaUs: state.vcpuCount * 100000,
        cpuPeriodUs: 100000,
        memoryBytes: (state.memSizeMib + CGROUP_VMM_OVERHEAD_MIB) * 1024 * 1024,
      };

      const spawnAndWait = async (timeoutMs: number): Promise<void> => {
        log.start("Spawning Firecracker via jailer...");
        consola.debug(
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
        const message = errorMessage(firstStartError);
        if (!isRecoverableStartError(message)) {
          logAttemptError("initial attempt", firstStartError);
          logDiagnostics();
          throw firstStartError;
        }

        logAttemptError("initial attempt", firstStartError);
        killOrphanVmProcess(vmId);
        if (existsSync(socketPath)) {
          unlinkSync(socketPath);
        }
        removeStaleDeviceNodes();
        removeStaleDevTrees();
        for (const rootDir of vmRootCandidates) {
          const staleFirecrackerBin = join(rootDir, "firecracker");
          if (existsSync(staleFirecrackerBin)) {
            unlinkSync(staleFirecrackerBin);
          }
          rmSync(join(rootDir, "firecracker.pid"), { force: true });
        }

        try {
          await spawnAndWait(15000);
          socketReady = true;
        } catch (retryError) {
          logAttemptError("retry attempt", retryError);
          logDiagnostics();
          throw new Error(
            `${startTag} retry failed after cleanup. First error: ${message}. Retry error: ${errorMessage(retryError)}`,
          );
        }
      }

      if (!socketReady) {
        throw new Error(`Timeout waiting for API socket at ${socketPath}`);
      }

      log.success("API socket ready");

      // 5. Boot VM
      const vm = new FirecrackerClient(socketPath);
      const bootArgs = NetworkManager.bootArgs(networkConfig.slot);
      consola.debug(`Boot args: ${bootArgs}`);
      await vm.boot("kernel/vmlinux", bootArgs);
      await vm.addDrive("rootfs", "rootfs/rootfs.ext4", true, false);
      await vm.configure(state.vcpuCount, state.memSizeMib);
      await vm.addNetwork("eth0", networkConfig.tapDevice, networkConfig.macAddress);

      log.start("Starting VM...");
      await vm.start();

      // 6. Update state
      const pid = getVmPid(vmId);
      store.update(vmId, { status: "running", pid });
      log.success(`VM ${vmId} is running (PID: ${pid || "unknown"})`);

      cmdLog.set({
        vmId,
        pid,
        guestIp: networkConfig.guestIp,
        networking: networkConfig.networkPolicy,
      });
      cmdLog.emit();
    } catch (error) {
      if (lifecycle.vmId) {
        killOrphanVmProcess(lifecycle.vmId);
        markVmAsError(lifecycle.vmId, error, paths);
      }

      cleanupNetwork(lifecycle.networkConfig);

      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default startCommand as CommandDef;
