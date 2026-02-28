import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { existsSync, mkdirSync, statSync, writeFileSync } from "node:fs";
import { basename, join, resolve } from "node:path";
import { consola } from "consola";
import { vmsanPaths } from "../paths.ts";
import { createCommandLogger } from "../lib/logger/index.ts";
import { FileVmStateStore } from "../lib/vm-state.ts";
import { handleCommandError, vmNotFoundError } from "../errors/index.ts";
import { waitForAgent } from "./create/connect.ts";
import { AgentClient } from "../services/agent.ts";

const downloadCommand = defineCommand({
  meta: {
    name: "download",
    description: "Download a file from a running VM",
  },
  args: {
    vmId: {
      type: "positional",
      description: "VM ID to download from, followed by remote file path",
      required: true,
    },
    dest: {
      type: "string",
      alias: "d",
      description: "Local destination path (default: basename of remote path in cwd)",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("download");
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

      if (!state.agentToken) {
        consola.error("VM has no agent token. Cannot download files without the agent.");
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      const log = consola.withTag(args.vmId);
      const guestIp = state.network.guestIp;
      const port = state.agentPort || paths.agentPort;
      consola.debug(`Agent endpoint: ${guestIp}:${port}`);

      log.start("Waiting for agent...");
      await waitForAgent(guestIp, port);

      const agent = new AgentClient(`http://${guestIp}:${port}`, state.agentToken);

      // args._ includes all positionals (including vmId at index 0), so remote path is at index 1.
      const remotePath = args._[1] as string | undefined;
      if (!remotePath) {
        consola.error("Remote file path is required.");
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      consola.debug(`Remote path: ${remotePath}`);
      log.start(`Downloading ${remotePath}...`);
      const data = await agent.readFile(remotePath);

      if (data === null) {
        consola.error(`File not found on VM: ${remotePath}`);
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      let localPath: string;
      if (args.dest) {
        const resolved = resolve(args.dest);
        const isDir =
          args.dest.endsWith("/") ||
          (existsSync(resolved) && statSync(resolved).isDirectory());
        if (isDir) {
          mkdirSync(resolved, { recursive: true });
          localPath = join(resolved, basename(remotePath));
        } else {
          localPath = resolved;
        }
      } else {
        localPath = resolve(basename(remotePath));
      }
      writeFileSync(localPath, data);

      log.success(`Downloaded to ${localPath} (${data.length} bytes)`);
      cmdLog.set({ vmId: args.vmId, remotePath, localPath });
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default downloadCommand as CommandDef;
