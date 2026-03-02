import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { join } from "node:path";
import { createCommandLogger, createScopedLogger, initVmsanLogger } from "../lib/logger/index.ts";
import { handleCommandError } from "../errors/index.ts";
import { createVmsan } from "../context.ts";
import { createCommandArgs } from "./create/args.ts";
import { parseCreateInput, type CreateCommandRuntimeArgs } from "./create/input.ts";
import { buildCreateSummaryLines } from "./create/summary.ts";
import { waitForAgent } from "./create/connect.ts";
import { ShellSession } from "../lib/shell/index.ts";
import {
  parseImageReference,
  parseDiskSizeGb,
  parseDomains,
  parseBandwidth,
} from "./create/validation.ts";

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
  "no-seccomp"?: boolean;
  "no-pid-ns"?: boolean;
  "no-cgroup"?: boolean;
  "no-netns"?: boolean;
  bandwidth?: string;
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

    try {
      const vmsan = await createVmsan();
      const parsedInput = parseCreateInput(commandArgs, vmsan.paths);

      // Auto-select Docker image for runtimes that define a default
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

      if (parsedInput.domains.length > 0) {
        consola.warn(
          "Domain filtering is DNS-based best effort. Direct-IP and DoH traffic may bypass allow-lists.",
        );
      }

      const bandwidthMbit = parseBandwidth(commandArgs.bandwidth);

      const fromImage = commandArgs["from-image"]
        ? parseImageReference(commandArgs["from-image"])
        : undefined;

      const result = await vmsan.create({
        vcpus: parsedInput.vcpus,
        memMib: parsedInput.memMib,
        diskSizeGb: parsedInput.diskSizeGb,
        kernelPath: typeof commandArgs.kernel === "string" ? commandArgs.kernel : undefined,
        rootfsPath: typeof commandArgs.rootfs === "string" ? commandArgs.rootfs : undefined,
        fromImage,
        project: commandArgs.project || "",
        runtime: parsedInput.runtime,
        networkPolicy: parsedInput.networkPolicy,
        domains: parsedInput.domains,
        allowedCidrs: parsedInput.allowedCidrs,
        deniedCidrs: parsedInput.deniedCidrs,
        ports: parsedInput.ports,
        bandwidthMbit,
        disableNetns: commandArgs["no-netns"],
        disableSeccomp: commandArgs["no-seccomp"],
        disablePidNs: commandArgs["no-pid-ns"],
        disableCgroup: commandArgs["no-cgroup"],
        timeoutMs: parsedInput.timeoutMs ?? undefined,
        snapshotId: parsedInput.snapshotId ?? undefined,
      });

      const log = createScopedLogger(result.vmId);
      const state = result.state;

      const summaryLines = buildCreateSummaryLines({
        vmId: result.vmId,
        pid: result.pid,
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
        kernelPath: state.kernel,
        rootfsPath: state.rootfs,
        snapshotId: parsedInput.snapshotId,
        timeout: commandArgs.timeout,
        socketPath: state.apiSocket,
        chrootDir: state.chrootDir,
        tapDevice: state.network.tapDevice,
        hostIp: state.network.hostIp,
        guestIp: state.network.guestIp,
        macAddress: state.network.macAddress,
        stateFilePath: join(vmsan.paths.vmsDir, `${result.vmId}.json`),
      });
      log.box(summaryLines.join("\n"));

      cmdLog.set({
        vmId: result.vmId,
        vcpus: parsedInput.vcpus,
        memMib: parsedInput.memMib,
        runtime: parsedInput.runtime,
        networkPolicy: parsedInput.networkPolicy,
        guestIp: state.network.guestIp,
        pid: result.pid,
        kernel: state.kernel,
        rootfs: state.rootfs,
        diskSizeGb: parsedInput.diskSizeGb,
      });
      cmdLog.emit();

      if (commandArgs.connect && state.agentToken) {
        log.start("Waiting for agent to become ready...");
        await waitForAgent(state.network.guestIp, vmsan.paths.agentPort);
        log.success("Agent is ready. Connecting via PTY shell...");

        const shell = new ShellSession({
          host: state.network.guestIp,
          port: vmsan.paths.agentPort,
          token: state.agentToken,
        });

        const closeInfo = await shell.connect();

        if (!closeInfo.sessionDestroyed && shell.sessionId) {
          const dim = "\x1b[2m";
          const reset = "\x1b[0m";
          process.stderr.write(
            `\n${dim}Resume this session with:\n  vmsan connect ${result.vmId} --session ${shell.sessionId}${reset}\n`,
          );
        }
      }
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export { parseDiskSizeGb, parseDomains, parseBandwidth };

export default createCommand as CommandDef;
