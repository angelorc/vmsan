import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { vmsanPaths } from "../paths.ts";
import { createCommandLogger } from "../lib/logger/index.ts";
import { FileVmStateStore } from "../lib/vm-state.ts";
import { handleCommandError, vmNotFoundError } from "../errors/index.ts";
import { waitForAgent } from "./create/connect.ts";
import { ShellSession } from "../lib/shell/index.ts";

const connectCommand = defineCommand({
  meta: {
    name: "connect",
    description: "Connect to a running VM",
  },
  args: {
    vmId: {
      type: "positional",
      description: "VM ID to connect to",
      required: true,
    },
    session: {
      type: "string",
      alias: "s",
      description: "Attach to an existing shell session ID",
      required: false,
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("connect");
    const paths = vmsanPaths();

    try {
      const store = new FileVmStateStore(paths.vmsDir);
      const state = store.load(args.vmId);

      if (!state) {
        throw vmNotFoundError(args.vmId);
      }

      if (state.status !== "running") {
        consola.error(`VM ${args.vmId} is not running (status: ${state.status})`);
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      const log = consola.withTag(args.vmId);
      const guestIp = state.network.guestIp;
      const port = state.agentPort || paths.agentPort;
      consola.debug(`Agent endpoint: ${guestIp}:${port}`);

      if (!state.agentToken) {
        consola.error(
          `VM ${args.vmId} has no agent token. The agent is required for shell access.`,
        );
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      log.start("Waiting for agent to become ready...");
      await waitForAgent(guestIp, port);
      log.success("Agent is ready. Connecting via PTY shell...");

      const shell = new ShellSession({
        host: guestIp,
        port,
        token: state.agentToken,
        sessionId: args.session,
      });

      const closeInfo = await shell.connect();

      cmdLog.set({ vmId: args.vmId, method: "pty" });
      cmdLog.emit();

      if (!closeInfo.sessionDestroyed && shell.sessionId) {
        const dim = "\x1b[2m";
        const reset = "\x1b[0m";
        process.stderr.write(
          `\n${dim}Resume this session with:\n  vmsan connect ${args.vmId} --session ${shell.sessionId}${reset}\n`,
        );
      }

      process.exit(0);
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exit(1);
    }
  },
});

export default connectCommand as CommandDef;
