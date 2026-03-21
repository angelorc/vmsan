import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createCommandLogger } from "../../lib/logger/index.ts";
import { handleCommandError } from "../../errors/index.ts";
import { ServerClient } from "../../lib/server-client.ts";

const removeCommand = defineCommand({
  meta: {
    name: "remove",
    description: "Remove a registered host",
  },
  args: {
    name: {
      type: "positional",
      description: "Name of the host to remove",
      required: true,
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("hosts remove");
    const log = consola.withTag("hosts");

    try {
      const client = ServerClient.fromEnv();

      log.start(`Looking up host "${args.name}"...`);
      const host = await client.findHostByName(args.name);

      if (!host) {
        consola.error(`Host "${args.name}" not found.`);
        cmdLog.set({ name: args.name, found: false });
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      log.start(`Removing host "${args.name}"...`);
      await client.deleteHost(host.id);
      log.success(`Host "${args.name}" removed.`);

      cmdLog.set({ name: args.name, id: host.id });
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default removeCommand as CommandDef;
