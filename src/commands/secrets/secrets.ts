import type { CommandDef } from "citty";
import { defineCommand } from "citty";

const secretsCommand = defineCommand({
  meta: {
    name: "secrets",
    description: "Manage project secrets",
  },
  subCommands: {
    set: () => import("./set.ts").then((m) => m.default),
    list: () => import("./list.ts").then((m) => m.default),
    ls: () => import("./list.ts").then((m) => m.default),
    unset: () => import("./unset.ts").then((m) => m.default),
  },
});

export default secretsCommand as CommandDef;
