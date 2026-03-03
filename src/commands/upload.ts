import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { readFileSync } from "node:fs";
import { basename } from "node:path";
import { consola } from "consola";
import { vmsanPaths } from "../paths.ts";
import { createCommandLogger } from "../lib/logger/index.ts";
import { handleCommandError } from "../errors/index.ts";
import { resolveVmState, waitForAgent } from "../lib/vm-context.ts";
import { AgentClient } from "../services/agent.ts";

const uploadCommand = defineCommand({
  meta: {
    name: "upload",
    description: "Upload local files to a running VM",
  },
  args: {
    vmId: {
      type: "positional",
      description: "VM ID to upload files to, followed by file path(s)",
      required: true,
    },
    dest: {
      type: "string",
      alias: "d",
      default: "/root",
      description: "Destination directory inside the VM (default: /root)",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("upload");
    const paths = vmsanPaths();

    try {
      const { state, guestIp, port } = resolveVmState(args.vmId, paths);
      const log = consola.withTag(args.vmId);
      consola.debug(`Agent endpoint: ${guestIp}:${port}`);

      log.start("Waiting for agent...");
      await waitForAgent(guestIp, port);

      // args._ includes all positionals (including vmId at index 0), so skip the first.
      const filePaths = args._.slice(1).filter(Boolean);
      if (filePaths.length === 0) {
        consola.error("No file paths provided.");
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      const files = filePaths.map((p: string) => ({
        path: basename(p),
        content: readFileSync(p),
      }));
      consola.debug(`File sizes: ${files.map((f) => `${f.path}=${f.content.length}b`).join(", ")}`);

      const agent = new AgentClient(`http://${guestIp}:${port}`, state.agentToken);

      log.start(`Uploading ${files.length} file(s) to ${args.dest}...`);
      await agent.writeFiles(files, args.dest);

      log.success(`Uploaded ${files.length} file(s) to ${args.dest}`);
      cmdLog.set({ vmId: args.vmId, files: filePaths, dest: args.dest });
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default uploadCommand as CommandDef;
