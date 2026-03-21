import { FileVmStateStore, type VmStateStore } from "../vm-state.ts";
import { SqliteVmStateStore } from "./sqlite.ts";
import { join } from "node:path";

export function createStateStore(vmsDir: string): VmStateStore {
  const backend = process.env.VMSAN_STATE_BACKEND ?? "sqlite";

  if (backend === "json") {
    return new FileVmStateStore(vmsDir);
  }

  // Derive the base directory from vmsDir (which is <baseDir>/vms)
  const baseDir = join(vmsDir, "..");
  const dbPath = join(baseDir, "state.db");
  return new SqliteVmStateStore(dbPath);
}

export { SqliteVmStateStore } from "./sqlite.ts";
export type { HostState, SyncLogEntry } from "./types.ts";
