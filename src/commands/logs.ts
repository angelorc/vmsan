import type { CommandDef } from "citty";
import { defineCommand } from "citty";

const logsCommand = defineCommand({
  meta: {
    name: "logs",
    description: "View VM logs",
  },
  subCommands: {
    dns: () => import("./logs/dns.ts").then((m) => m.default),
  },
});

export default logsCommand as CommandDef;
