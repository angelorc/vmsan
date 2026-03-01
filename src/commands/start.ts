import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { createCommandLogger } from "../lib/logger/index.ts";
import { handleCommandError } from "../errors/index.ts";
import { createVmsan } from "../context.ts";

const startCommand = defineCommand({
  meta: {
    name: "start",
    description: "Start a previously stopped VM",
  },
  args: {
    vmId: {
      type: "positional",
      description: "VM ID to start",
      required: true,
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("start");

    try {
      const vmId = args.vmId as string;
      const vmsan = await createVmsan();

      const result = await vmsan.start(vmId);

      if (!result.success) {
        if (result.error) throw result.error;
        process.exitCode = 1;
        cmdLog.emit();
        return;
      }

      const state = vmsan.get(vmId);
      cmdLog.set({
        vmId,
        pid: result.pid,
        guestIp: state?.network.guestIp,
        networking: state?.network.networkPolicy,
      });
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default startCommand as CommandDef;
