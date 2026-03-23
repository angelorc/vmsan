import { consola } from "consola";
import type { VmsanPaths } from "../../paths.ts";
import { FileVmStateStore } from "../../lib/vm-state.ts";
import { toError } from "../../lib/utils.ts";

export function markVmAsError(vmId: string, error: unknown, paths: VmsanPaths): void {
  try {
    const store = new FileVmStateStore(paths.vmsDir);
    store.update(vmId, {
      status: "error",
      error: toError(error).message,
    });
  } catch (err) {
    consola.debug(`Failed to mark VM ${vmId} as error: ${toError(err).message}`);
  }
}
