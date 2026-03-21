import type { CommandDef } from "citty";
import { defineCommand, runCommand } from "citty";

const SUBCOMMAND_NAMES = new Set(["dns"]);

const logsCommand = defineCommand({
  meta: {
    name: "logs",
    description: "Stream application and DNS logs",
  },
  args: {
    service: {
      type: "positional",
      description: "Service name to filter (omit for all services)",
      required: false,
    },
    lines: {
      type: "string",
      alias: "n",
      default: "100",
      description: "Number of historical lines to show",
    },
    follow: {
      type: "boolean",
      alias: "f",
      default: true,
      description: "Follow log output (stream mode)",
    },
    timestamps: {
      type: "boolean",
      alias: "t",
      default: false,
      description: "Show timestamps in log output",
    },
    config: {
      type: "string",
      description: "Path to vmsan.toml (default: ./vmsan.toml)",
    },
  },
  subCommands: {
    dns: () => import("./logs/dns.ts").then((m) => m.default),
  },
  async run({ rawArgs, args }) {
    // When citty dispatches a subcommand (e.g. "dns"), it still calls the
    // parent's run(). Skip if the first positional matches a subcommand name.
    if (args.service && SUBCOMMAND_NAMES.has(args.service)) {
      return;
    }

    // Delegate to the app logs command, forwarding raw args
    const appCmd = await import("./logs/app.ts").then((m) => m.default);
    await runCommand(appCmd, { rawArgs });
  },
});

export default logsCommand as CommandDef;
