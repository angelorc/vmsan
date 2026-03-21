import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { resolve, basename } from "node:path";
import { existsSync } from "node:fs";
import { createVmsan } from "../../context.ts";
import { loadVmsanToml } from "../../lib/toml/parser.ts";
import { orchestrateDeploy } from "../../lib/deploy/orchestrator.ts";
import { handleCommandError } from "../../errors/index.ts";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { table } from "../../lib/utils.ts";

const upCommand = defineCommand({
  meta: {
    name: "up",
    description: "Deploy services defined in vmsan.toml",
  },
  args: {
    config: {
      type: "string",
      description: "Path to vmsan.toml (default: ./vmsan.toml)",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("up");

    try {
      // 1. Find vmsan.toml
      const configPath = resolve(args.config || "vmsan.toml");
      if (!existsSync(configPath)) {
        consola.error(`Configuration file not found: ${configPath}`);
        consola.info('Run "vmsan init" to create a vmsan.toml');
        process.exitCode = 1;
        return;
      }

      // 2. Load and parse
      consola.start(`Loading ${configPath}`);
      const config = loadVmsanToml(configPath);

      // 3. Determine project name from directory name
      const sourceDir = resolve(configPath, "..");
      const project = basename(sourceDir);

      // 4. Create VMService
      const vmService = await createVmsan();

      // 5. Orchestrate deployment
      consola.info(`Deploying project "${project}"`);
      const result = await orchestrateDeploy({
        config,
        project,
        sourceDir,
        vmService,
        env: {},
      });

      // 6. Display results
      if (getOutputMode() === "json") {
        cmdLog.set({
          project,
          success: result.success,
          durationMs: result.durationMs,
          services: result.services,
        });
        cmdLog.emit();
      } else {
        consola.log("");
        if (result.services.length > 0) {
          const statusColor = (status: string): string => {
            if (status === "running") return `\x1b[32m${status}\x1b[0m`;
            if (status === "skipped") return `\x1b[33m${status}\x1b[0m`;
            if (status === "failed") return `\x1b[31m${status}\x1b[0m`;
            return status;
          };

          consola.log(
            table({
              rows: result.services,
              columns: {
                SERVICE: { value: (r) => r.service },
                STATUS: {
                  value: (r) => r.status,
                  color: (r) => statusColor(r.status),
                },
                VM: { value: (r) => r.vmId ?? "-" },
                DURATION: { value: (r) => `${Math.round(r.durationMs / 1000)}s` },
              },
            }),
          );
          consola.log("");
        }

        const totalSeconds = Math.round(result.durationMs / 1000);
        if (result.success) {
          consola.success(`Deployed in ${totalSeconds}s`);
        } else {
          consola.error(`Deploy failed after ${totalSeconds}s`);
          for (const svc of result.services) {
            if (svc.error) {
              consola.error(`  ${svc.service}: ${svc.error}`);
            }
          }
          process.exitCode = 1;
        }
      }
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default upCommand as CommandDef;
