import type { CommandDef } from "citty";
import { defineCommand, runCommand } from "citty";

const logsCommand = defineCommand({
  meta: {
    name: "logs",
    description: "Stream application and DNS logs",
  },
  args: {
    service: {
      type: "positional",
      description: 'Service name to filter, or "dns" for DNS logs',
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
  async run({ rawArgs, args }) {
    // Route "vmsan logs dns ..." to the DNS logs subcommand.
    if (args.service === "dns") {
      const dnsCmd = await import("./logs/dns.ts").then((m) => m.default);
      await runCommand(dnsCmd, { rawArgs: rawArgs.filter((a) => a !== "dns") });
      return;
    }

    // Delegate to the app logs command, forwarding raw args
    const appCmd = await import("./logs/app.ts").then((m) => m.default);
    await runCommand(appCmd, { rawArgs });
  },
});

export default logsCommand as CommandDef;
