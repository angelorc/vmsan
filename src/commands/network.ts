import type { CommandDef } from "citty";
import { defineCommand } from "citty";

const networkCommand = defineCommand({
  meta: {
    name: "network",
    description: "Manage VM networking",
  },
  subCommands: {
    update: () => import("./network/update.ts").then((m) => m.default),
    connections: () => import("./network/connections.ts").then((m) => m.default),
  },
});

export default networkCommand as CommandDef;
