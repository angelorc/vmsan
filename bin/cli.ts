#!/usr/bin/env node

// Suppress DEP0040 (punycode) from cloudflare SDK's transitive dep chain:
// cloudflare → node-fetch → whatwg-url → tr46 → require("punycode")
const _origEmit = process.emit;
process.emit = function (event: string, ...args: unknown[]) {
  if (
    event === "warning" &&
    (args[0] as { name?: string })?.name === "DeprecationWarning" &&
    (args[0] as { code?: string })?.code === "DEP0040"
  ) {
    return false;
  }
  return _origEmit.apply(process, [event, ...args] as Parameters<typeof _origEmit>);
};

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

const SUDO_COMMANDS = new Set([
  "create",
  "start",
  "stop",
  "remove",
  "rm",
  "snapshot",
  "up",
  "deploy",
  "down",
]);

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
    exec: () => import("../src/commands/exec.ts").then((m) => m.default),
    network: () => import("../src/commands/network.ts").then((m) => m.default),
    snapshot: () => import("../src/commands/snapshot.ts").then((m) => m.default),
    logs: () => import("../src/commands/logs.ts").then((m) => m.default),
    secrets: () => import("../src/commands/secrets/secrets.ts").then((m) => m.default),
    doctor: () => import("../src/commands/doctor.ts").then((m) => m.default),
    init: () => import("../src/commands/init.ts").then((m) => m.default),
    validate: () => import("../src/commands/validate.ts").then((m) => m.default),
    up: () => import("../src/commands/up/up.ts").then((m) => m.default),
    deploy: () => import("../src/commands/deploy/deploy.ts").then((m) => m.default),
    status: () => import("../src/commands/status/status.ts").then((m) => m.default),
    down: () => import("../src/commands/down/down.ts").then((m) => m.default),
    migrate: () => import("../src/commands/migrate/migrate.ts").then((m) => m.default),
    hosts: () => import("../src/commands/hosts/hosts.ts").then((m) => m.default),
  },
});

runMain(main);
