import type { CommandDef } from "citty";
import { defineCommand } from "citty";

const snapshotCommand = defineCommand({
  meta: {
    name: "snapshot",
    description: "Manage VM snapshots",
  },
  subCommands: {
    create: () => import("./snapshot/create.ts").then((m) => m.default),
    list: () => import("./snapshot/list.ts").then((m) => m.default),
    ls: () => import("./snapshot/list.ts").then((m) => m.default),
    delete: () => import("./snapshot/delete.ts").then((m) => m.default),
    rm: () => import("./snapshot/delete.ts").then((m) => m.default),
  },
});

export default snapshotCommand as CommandDef;
