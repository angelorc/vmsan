import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { randomBytes } from "node:crypto";
import { existsSync } from "node:fs";
import { spawn } from "node:child_process";
import { join } from "node:path";
import { vmsanPaths } from "../paths.ts";
import { createCommandLogger, createScopedLogger, initVmsanLogger } from "../lib/logger/index.ts";
import {
  mutuallyExclusiveFlagsError,
  handleCommandError,
} from "../errors/index.ts";
import { generateVmId } from "../lib/utils.ts";
import { FileVmStateStore } from "../lib/vm-state.ts";
import { FirecrackerClient } from "../services/firecracker.ts";
import { Jailer } from "../lib/jailer.ts";
import { NetworkManager } from "../lib/network.ts";
import { createCommandArgs } from "./create/args.ts";
import {
  cleanupChroot,
  cleanupNetwork,
  killOrphanVmProcess,
  markVmAsError,
} from "./create/cleanup.ts";
import {
  findKernel,
  findRootfs,
  getVmPid,
  validateEnvironment,
  waitForSocket,
} from "./create/environment.ts";
import { parseCreateInput, type CreateCommandRuntimeArgs } from "./create/input.ts";
import { buildCreateSummaryLines } from "./create/summary.ts";
import { buildInitialVmState } from "./create/state.ts";
import type { CreateLifecycleState } from "./create/types.ts";
import { resolveImageRootfs } from "./create/image-rootfs.ts";
import { parseDiskSizeGb, parseDomains, parseImageReference } from "./create/validation.ts";
import { waitForAgent } from "./create/connect.ts";
import { ShellSession } from "../lib/shell/index.ts";

const RUNTIME_DEFAULT_IMAGES: Record<string, string> = {
  "node22-demo": "node:22",
};

interface CreateCommandArgs extends CreateCommandRuntimeArgs {
  kernel?: string;
  rootfs?: string;
  "from-image"?: string;
  project?: string;
  timeout?: string;
  silent?: boolean;
  connect?: boolean;
}

function createLifecycleState(): CreateLifecycleState {
  return {
    networkConfig: undefined,
    vmId: undefined,
    chrootDir: undefined,
  };
}

const createCommand = defineCommand({
  meta: {
    name: "create",
    description: "Create and start a Firecracker microVM",
  },
  args: createCommandArgs,
  async run({ args }) {
    const commandArgs = args as CreateCommandArgs;
    if (commandArgs.silent) {
      initVmsanLogger("silent");
    }

    const cmdLog = createCommandLogger("create");
    const lifecycle = createLifecycleState();
    const paths = vmsanPaths();
    const store = new FileVmStateStore(paths.vmsDir);

    try {
      const baseDir = paths.baseDir;
      validateEnvironment(baseDir);

      if (commandArgs["from-image"] && commandArgs.rootfs) {
        throw mutuallyExclusiveFlagsError("--from-image", "--rootfs");
      }

      // Auto-select Docker image for runtimes that define a default
      const parsedInput = parseCreateInput(commandArgs, paths);
      const defaultImage = RUNTIME_DEFAULT_IMAGES[parsedInput.runtime];
      if (defaultImage && !commandArgs["from-image"] && !commandArgs.rootfs) {
        commandArgs["from-image"] = defaultImage;
        consola.info(`Runtime "${parsedInput.runtime}" auto-selected image: ${defaultImage}`);
      }

      if (parsedInput.runtime === "node22-demo" && parsedInput.ports.length === 0) {
        consola.warn(
          "Runtime node22-demo serves a welcome page, but no --publish-port was specified. The page won't be accessible externally.",
        );
      }

      const kernelPath =
        typeof commandArgs.kernel === "string" ? commandArgs.kernel : findKernel(baseDir);
      consola.debug(`Kernel resolved: ${kernelPath}`);

      let rootfsPath: string;
      if (typeof commandArgs["from-image"] === "string") {
        const imageRef = parseImageReference(commandArgs["from-image"]);
        rootfsPath = resolveImageRootfs(imageRef, paths.registryDir);
      } else {
        rootfsPath =
          typeof commandArgs.rootfs === "string" ? commandArgs.rootfs : findRootfs(baseDir);
      }
      consola.debug(`Rootfs resolved: ${rootfsPath}`);

      if (parsedInput.domains.length > 0) {
        consola.warn(
          "Domain filtering is DNS-based best effort. Direct-IP and DoH traffic may bypass allow-lists.",
        );
      }

      lifecycle.vmId = generateVmId();
      const log = createScopedLogger(lifecycle.vmId);
      const slot = store.allocateNetworkSlot();
      consola.debug(`Network slot allocated: ${slot}`);
      const net = new NetworkManager(
        slot,
        parsedInput.networkPolicy,
        parsedInput.domains,
        parsedInput.allowedCidrs,
        parsedInput.deniedCidrs,
        parsedInput.ports,
      );
      lifecycle.networkConfig = net.config;

      log.start(`Creating VM ${lifecycle.vmId}...`);

      // Generate agent token if binary is available.
      const agentToken = existsSync(paths.agentBin) ? randomBytes(32).toString("hex") : null;

      const state = buildInitialVmState({
        vmId: lifecycle.vmId,
        project: commandArgs.project || "",
        runtime: parsedInput.runtime,
        diskSizeGb: parsedInput.diskSizeGb,
        kernelPath,
        rootfsPath,
        vcpus: parsedInput.vcpus,
        memMib: parsedInput.memMib,
        networkPolicy: parsedInput.networkPolicy,
        domains: parsedInput.domains,
        allowedCidrs: parsedInput.allowedCidrs,
        deniedCidrs: parsedInput.deniedCidrs,
        ports: parsedInput.ports,
        tapDevice: lifecycle.networkConfig.tapDevice,
        hostIp: lifecycle.networkConfig.hostIp,
        guestIp: lifecycle.networkConfig.guestIp,
        subnetMask: lifecycle.networkConfig.subnetMask,
        macAddress: lifecycle.networkConfig.macAddress,
        snapshotId: parsedInput.snapshotId,
        timeoutMs: parsedInput.timeoutMs,
        agentToken,
        agentPort: paths.agentPort,
      });
      store.save(state);

      log.start("Setting up networking...");
      await net.setup();
      log.success(
        `Network: TAP ${lifecycle.networkConfig.tapDevice}, Host ${lifecycle.networkConfig.hostIp}, Guest ${lifecycle.networkConfig.guestIp}`,
      );

      log.start("Preparing chroot...");
      const snapshotConfig = parsedInput.snapshotId
        ? {
            snapshotFile: join(paths.snapshotsDir, parsedInput.snapshotId, "snapshot_file"),
            memFile: join(paths.snapshotsDir, parsedInput.snapshotId, "mem_file"),
          }
        : undefined;

      const jailer = new Jailer(lifecycle.vmId, paths.jailerBaseDir);
      const welcomePage =
        parsedInput.runtime === "node22-demo" && parsedInput.ports.length > 0
          ? { vmId: lifecycle.vmId, ports: parsedInput.ports }
          : undefined;

      const agentConfig = agentToken
        ? {
            binaryPath: paths.agentBin,
            token: agentToken,
            port: paths.agentPort,
            vmId: lifecycle.vmId,
          }
        : undefined;

      const jailerPaths = jailer.prepare({
        kernelSrc: kernelPath,
        rootfsSrc: rootfsPath,
        diskSizeGb: parsedInput.diskSizeGb,
        snapshot: snapshotConfig,
        welcomePage,
        agent: agentConfig,
      });
      lifecycle.chrootDir = jailerPaths.chrootDir;

      store.update(lifecycle.vmId, {
        chrootDir: jailerPaths.chrootDir,
        apiSocket: jailerPaths.socketPath,
      });
      consola.debug(`Jailer chroot: ${jailerPaths.chrootDir}`);
      consola.debug(`API socket path: ${jailerPaths.socketPath}`);

      log.start("Spawning Firecracker via jailer...");
      const firecrackerBin = join(baseDir, "bin", "firecracker");
      const jailerBin = join(baseDir, "bin", "jailer");

      jailer.spawn({
        firecrackerBin,
        jailerBin,
        chrootBase: jailerPaths.chrootBase,
      });

      log.start("Waiting for API socket...");
      await waitForSocket(jailerPaths.socketPath, 5000);
      log.success("API socket ready");

      const vm = new FirecrackerClient(jailerPaths.socketPath);

      if (parsedInput.snapshotId) {
        log.start("Restoring from snapshot...");
        await vm.loadSnapshot("snapshot/snapshot_file", "snapshot/mem_file");
        await vm.resume();
        log.success("Snapshot restored and VM resumed");
      } else {
        const bootArgs = NetworkManager.bootArgs(slot);
        consola.debug(`Boot args: ${bootArgs}`);
        await vm.boot("kernel/vmlinux", bootArgs);
        await vm.addDrive("rootfs", "rootfs/rootfs.ext4", true, false);
        await vm.configure(parsedInput.vcpus, parsedInput.memMib);
        await vm.addNetwork(
          "eth0",
          lifecycle.networkConfig.tapDevice,
          lifecycle.networkConfig.macAddress,
        );

        log.start("Starting VM...");
        await vm.start();
      }

      const pid = getVmPid(lifecycle.vmId);
      consola.debug(`Firecracker PID: ${pid ?? "unknown"}`);
      store.update(lifecycle.vmId, { status: "running", pid });
      log.success(`VM ${lifecycle.vmId} is running (PID: ${pid || "unknown"})`);

      if (parsedInput.timeoutMs && pid) {
        const killer = spawn(
          "bash",
          [
            "-c",
            `sleep ${Math.ceil(parsedInput.timeoutMs / 1000)} && [ -d /proc/${pid} ] && grep -q "${lifecycle.vmId}" /proc/${pid}/cmdline 2>/dev/null && kill ${pid} 2>/dev/null`,
          ],
          { detached: true, stdio: "ignore" },
        );
        killer.unref();
      }

      const summaryLines = buildCreateSummaryLines({
        vmId: lifecycle.vmId,
        pid,
        vcpus: parsedInput.vcpus,
        memMib: parsedInput.memMib,
        runtime: parsedInput.runtime,
        diskSizeGb: parsedInput.diskSizeGb,
        project: commandArgs.project || "",
        networkPolicy: parsedInput.networkPolicy,
        domains: parsedInput.domains,
        allowedCidrs: parsedInput.allowedCidrs,
        deniedCidrs: parsedInput.deniedCidrs,
        ports: parsedInput.ports,
        kernelPath,
        rootfsPath,
        snapshotId: parsedInput.snapshotId,
        timeout: commandArgs.timeout,
        socketPath: jailerPaths.socketPath,
        chrootDir: jailerPaths.chrootDir,
        tapDevice: lifecycle.networkConfig.tapDevice,
        hostIp: lifecycle.networkConfig.hostIp,
        guestIp: lifecycle.networkConfig.guestIp,
        macAddress: lifecycle.networkConfig.macAddress,
        stateFilePath: join(paths.vmsDir, `${lifecycle.vmId}.json`),
      });
      log.box(summaryLines.join("\n"));

      cmdLog.set({
        vmId: lifecycle.vmId,
        vcpus: parsedInput.vcpus,
        memMib: parsedInput.memMib,
        runtime: parsedInput.runtime,
        networkPolicy: parsedInput.networkPolicy,
        guestIp: lifecycle.networkConfig.guestIp,
        pid,
        kernel: kernelPath,
        rootfs: rootfsPath,
        diskSizeGb: parsedInput.diskSizeGb,
      });
      cmdLog.emit();

      if (commandArgs.connect && lifecycle.networkConfig && agentToken) {
        log.start("Waiting for agent to become ready...");
        await waitForAgent(lifecycle.networkConfig.guestIp, paths.agentPort);
        log.success("Agent is ready. Connecting via PTY shell...");

        const shell = new ShellSession({
          host: lifecycle.networkConfig.guestIp,
          port: paths.agentPort,
          token: agentToken,
        });

        await shell.connect();
      }
    } catch (error) {
      if (lifecycle.vmId) {
        killOrphanVmProcess(lifecycle.vmId);
        markVmAsError(lifecycle.vmId, error, paths);
      }

      cleanupNetwork(lifecycle.networkConfig);
      cleanupChroot(lifecycle.chrootDir);

      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export { parseDiskSizeGb, parseDomains };

export default createCommand as CommandDef;
