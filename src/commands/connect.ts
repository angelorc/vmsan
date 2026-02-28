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

      // Start connection; session ID arrives via text frame before shell I/O.
      const connectPromise = shell.connect();

      // Poll briefly for session ID so we can log it before interactive use.
      const deadline = Date.now() + 2000;
      while (!shell.sessionId && Date.now() < deadline) {
        await new Promise((r) => setTimeout(r, 50));
      }
      if (shell.sessionId) {
        consola.debug(`Shell session established: ${shell.sessionId}`);
        log.info(`Session ID: ${shell.sessionId} (use --session to reattach)`);
      }

      await connectPromise;

      cmdLog.set({ vmId: args.vmId, method: "pty" });
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default connectCommand as CommandDef;
