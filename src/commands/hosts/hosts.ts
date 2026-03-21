import type { CommandDef } from "citty";
import { defineCommand } from "citty";

const hostsCommand = defineCommand({
  meta: {
    name: "hosts",
    description: "Manage remote hosts",
  },
  subCommands: {
    add: () => import("./add.ts").then((m) => m.default),
    list: () => import("./list.ts").then((m) => m.default),
    ls: () => import("./list.ts").then((m) => m.default),
    remove: () => import("./remove.ts").then((m) => m.default),
    rm: () => import("./remove.ts").then((m) => m.default),
    check: () => import("./check.ts").then((m) => m.default),
  },
});

export default hostsCommand as CommandDef;
