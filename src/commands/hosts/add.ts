import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createCommandLogger } from "../../lib/logger/index.ts";
import { handleCommandError } from "../../errors/index.ts";
import { ServerClient } from "../../lib/server-client.ts";

const addCommand = defineCommand({
  meta: {
    name: "add",
    description: "Generate a join command for a remote host",
  },
  args: {
    name: {
      type: "positional",
      description: "Name for the new host",
      required: true,
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("hosts add");
    const log = consola.withTag("hosts");

    try {
      const client = ServerClient.fromEnv();

      log.start("Generating join token...");
      const { token, expires_at } = await client.generateToken();

      const serverUrl = process.env.VMSAN_SERVER_URL ?? "http://10.88.0.1:6443";
      const joinCmd = `vmsan agent join --server ${serverUrl} --token ${token}`;

      log.success(`Run this command on ${args.name}:`);
      log.log("");
      log.log(`  ${joinCmd}`);
      log.log("");
      log.log(`Token expires: ${expires_at}`);

      cmdLog.set({ name: args.name, token, expires_at, command: joinCmd });
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default addCommand as CommandDef;
