#!/usr/bin/env node

import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { defineCommand, runMain } from "citty";
import { consola } from "consola";
import { initVmsanLogger } from "../src/lib/logger/index.ts";

function findPackageVersion(): string {
  let dir = dirname(fileURLToPath(import.meta.url));
  while (dir !== dirname(dir)) {
    const pkgPath = join(dir, "package.json");
    if (existsSync(pkgPath)) {
      return JSON.parse(readFileSync(pkgPath, "utf8")).version;
    }
    dir = dirname(dir);
  }
  return "0.0.0";
}

const version = findPackageVersion();

const SUDO_COMMANDS = new Set(["create", "start", "stop", "remove", "rm"]);

const subCommand = process.argv[2];
if (subCommand && SUDO_COMMANDS.has(subCommand) && process.getuid?.() !== 0) {
  consola.error(
    `"vmsan ${subCommand}" requires root. Run with: sudo env "PATH=$PATH" vmsan ${subCommand}`,
  );
  process.exit(1);
}

const main = defineCommand({
  meta: {
    name: "vmsan",
    version,
    description: "Firecracker microVM sandbox toolkit",
  },
  args: {
    json: {
      type: "boolean",
      default: false,
      description: "Output structured JSON (one event per command)",
    },
    verbose: {
      type: "boolean",
      default: false,
      description: "Show detailed debug output with wide event tree",
    },
  },
  setup({ args }) {
    const mode = args.json ? "json" : args.verbose ? "verbose" : "normal";
    initVmsanLogger(mode);
  },
  subCommands: {
    create: () => import("../src/commands/create.ts").then((m) => m.default),
    list: () => import("../src/commands/list.ts").then((m) => m.default),
    ls: () => import("../src/commands/list.ts").then((m) => m.default),
    start: () => import("../src/commands/start.ts").then((m) => m.default),
    stop: () => import("../src/commands/stop.ts").then((m) => m.default),
    remove: () => import("../src/commands/remove.ts").then((m) => m.default),
    rm: () => import("../src/commands/remove.ts").then((m) => m.default),
    connect: () => import("../src/commands/connect.ts").then((m) => m.default),
    upload: () => import("../src/commands/upload.ts").then((m) => m.default),
    download: () => import("../src/commands/download.ts").then((m) => m.default),
    network: () => import("../src/commands/network.ts").then((m) => m.default),
  },
});

runMain(main);
