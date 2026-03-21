import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { existsSync, writeFileSync } from "node:fs";
import { basename, join, resolve } from "node:path";
import { consola } from "consola";
import { stringify } from "smol-toml";
import { detectProject } from "../lib/toml/detect.ts";
import type { VmsanToml } from "../lib/toml/types.ts";
import { VALID_RUNTIMES } from "./create/types.ts";

const kCancel = Symbol.for("cancel");

function isCancelled(value: unknown): boolean {
  return value === kCancel;
}

function buildToml(config: VmsanToml): string {
  const obj: Record<string, unknown> = {};

  if (config.project) {
    obj.project = config.project;
  }

  if (config.services) {
    obj.services = config.services;
  }

  if (config.accessories) {
    obj.accessories = config.accessories;
  }

  if (config.tunnel) {
    obj.tunnel = config.tunnel;
  }

  return stringify(obj);
}

const initCommand = defineCommand({
  meta: {
    name: "init",
    description: "Initialize a vmsan.toml configuration file",
  },
  args: {
    yes: {
      type: "boolean",
      alias: "y",
      default: false,
      description: "Accept all defaults (non-interactive mode)",
    },
  },
  async run({ args }) {
    const cwd = resolve(".");
    const tomlPath = join(cwd, "vmsan.toml");

    if (existsSync(tomlPath)) {
      consola.warn("vmsan.toml already exists in this directory.");
      if (!args.yes) {
        const overwrite = await consola.prompt("Overwrite?", {
          type: "confirm",
          initial: false,
          cancel: "symbol",
        });
        if (isCancelled(overwrite) || !overwrite) {
          consola.info("Aborted.");
          return;
        }
      } else {
        consola.info("Overwriting existing vmsan.toml (--yes).");
      }
    }

    // Auto-detect project
    const detected = detectProject(cwd);
    const dirName = basename(cwd);

    if (detected) {
      consola.info(`Detected: ${detected.reason}`);
      if (detected.build) consola.info(`  Build: ${detected.build}`);
      if (detected.start) consola.info(`  Start: ${detected.start}`);
      consola.log("");
    }

    let runtime = detected?.runtime ?? "base";
    let buildCmd = detected?.build ?? "";
    let startCmd = detected?.start ?? "";
    let database: string = "none";
    let redis = false;
    let hostname = "";
    let projectName = dirName;

    if (!args.yes) {
      // Interactive mode

      const runtimeAnswer = await consola.prompt(`Runtime`, {
        type: "text",
        default: runtime,
        placeholder: runtime,
        cancel: "symbol",
      });
      if (isCancelled(runtimeAnswer)) {
        consola.info("Aborted.");
        return;
      }
      if (typeof runtimeAnswer === "string" && runtimeAnswer.trim()) {
        runtime = runtimeAnswer.trim();
      }

      // Validate runtime
      if (!VALID_RUNTIMES.includes(runtime as (typeof VALID_RUNTIMES)[number])) {
        consola.warn(`Unknown runtime "${runtime}". Valid runtimes: ${VALID_RUNTIMES.join(", ")}`);
      }

      const buildAnswer = await consola.prompt(`Build command`, {
        type: "text",
        default: buildCmd || "none",
        placeholder: buildCmd || "none",
        cancel: "symbol",
      });
      if (isCancelled(buildAnswer)) {
        consola.info("Aborted.");
        return;
      }
      if (typeof buildAnswer === "string") {
        buildCmd = buildAnswer.trim() === "none" ? "" : buildAnswer.trim();
      }

      const startAnswer = await consola.prompt(`Start command`, {
        type: "text",
        default: startCmd || "none",
        placeholder: startCmd || "none",
        cancel: "symbol",
      });
      if (isCancelled(startAnswer)) {
        consola.info("Aborted.");
        return;
      }
      if (typeof startAnswer === "string") {
        startCmd = startAnswer.trim() === "none" ? "" : startAnswer.trim();
      }

      const dbAnswer = await consola.prompt(`Database?`, {
        type: "select",
        options: ["none", "postgres", "mysql"],
        initial: "none",
        cancel: "symbol",
      });
      if (isCancelled(dbAnswer)) {
        consola.info("Aborted.");
        return;
      }
      if (typeof dbAnswer === "string") {
        database = dbAnswer;
      }

      const redisAnswer = await consola.prompt(`Redis?`, {
        type: "confirm",
        initial: false,
        cancel: "symbol",
      });
      if (isCancelled(redisAnswer)) {
        consola.info("Aborted.");
        return;
      }
      redis = redisAnswer === true;

      const hostnameAnswer = await consola.prompt(`Public hostname?`, {
        type: "text",
        default: "none",
        placeholder: "none",
        cancel: "symbol",
      });
      if (isCancelled(hostnameAnswer)) {
        consola.info("Aborted.");
        return;
      }
      if (typeof hostnameAnswer === "string") {
        hostname = hostnameAnswer.trim() === "none" ? "" : hostnameAnswer.trim();
      }

      const nameAnswer = await consola.prompt(`Project name`, {
        type: "text",
        default: projectName,
        placeholder: projectName,
        cancel: "symbol",
      });
      if (isCancelled(nameAnswer)) {
        consola.info("Aborted.");
        return;
      }
      if (typeof nameAnswer === "string" && nameAnswer.trim()) {
        projectName = nameAnswer.trim();
      }
    }

    // Build config
    const config: VmsanToml = {
      project: projectName,
      services: {},
      accessories: {},
    };

    // Build depends_on and connect_to lists
    const dependsOn: string[] = [];
    const connectTo: string[] = [];

    if (database !== "none") {
      config.accessories!.db = { type: database };
      dependsOn.push("db");
      const port = database === "postgres" ? "5432" : "3306";
      connectTo.push(`db:${port}`);
    }

    if (redis) {
      config.accessories!.redis = { type: "redis" };
      dependsOn.push("redis");
      connectTo.push("redis:6379");
    }

    config.services!.web = {
      runtime,
      ...(buildCmd ? { build: buildCmd } : {}),
      start: startCmd || "echo 'no start command configured'",
      ...(dependsOn.length > 0 ? { depends_on: dependsOn } : {}),
      ...(connectTo.length > 0 ? { connect_to: connectTo } : {}),
    };

    if (hostname) {
      config.tunnel = { hostname };
    }

    // Remove empty accessories
    if (Object.keys(config.accessories!).length === 0) {
      delete config.accessories;
    }

    const tomlContent = buildToml(config);

    writeFileSync(tomlPath, tomlContent + "\n", "utf-8");
    consola.success(`Created vmsan.toml`);
    consola.log("");
    consola.log(tomlContent);
  },
});

export default initCommand as CommandDef;
