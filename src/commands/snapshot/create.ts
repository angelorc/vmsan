import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createCommandLogger, getOutputMode } from "../../lib/logger/index.ts";
import { handleCommandError } from "../../errors/index.ts";
import { createVmsanContext } from "../../context.ts";
import { SnapshotService } from "../../services/snapshot.ts";

const snapshotCreateCommand = defineCommand({
  meta: {
    name: "create",
    description: "Create a snapshot of a running VM",
  },
  args: {
    vmId: {
      type: "positional",
      description: "VM ID to snapshot",
      required: true,
    },
    "no-resume": {
      type: "boolean",
      default: false,
      description: "Keep VM paused after snapshot (default: resume)",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("snapshot:create");

    try {
      const ctx = await createVmsanContext();
      const service = new SnapshotService(ctx);

      const result = await service.create({
        vmId: args.vmId,
        resume: !args["no-resume"],
      });

      if (getOutputMode() === "json") {
        cmdLog.set({
          snapshotId: result.snapshotId,
          vmId: result.vmId,
          snapshotPath: result.snapshotPath,
          memPath: result.memPath,
        });
      } else {
        consola.success(`Snapshot created: ${result.snapshotId}`);
      }

      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default snapshotCreateCommand as CommandDef;
