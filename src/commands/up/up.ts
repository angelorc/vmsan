import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createVmsan } from "../../context.ts";
import { orchestrateDeploy } from "../../lib/deploy/orchestrator.ts";
import { handleCommandError } from "../../errors/index.ts";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { table } from "../../lib/utils.ts";
import { loadProjectConfig } from "../../lib/project.ts";

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
      // 1. Load project config
      const { config, configPath, sourceDir, project } = loadProjectConfig(args.config);
      consola.start(`Loading ${configPath}`);

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
