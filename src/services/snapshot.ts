import {
  existsSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
} from "node:fs";
import { join } from "node:path";
import type { VmsanContext } from "../context.ts";
import {
  vmNotFoundError,
  vmNotRunningError,
  snapshotNotFoundError,
  snapshotCreateFailedError,
} from "../errors/index.ts";
import { mkdirSecure, writeSecure, toError } from "../lib/utils.ts";
import { GatewayClient } from "../lib/gateway-client.ts";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface CreateSnapshotOptions {
  vmId: string;
  resume?: boolean;
}

export interface CreateSnapshotResult {
  snapshotId: string;
  vmId: string;
  snapshotPath: string;
  memPath: string;
}

export interface SnapshotEntry {
  id: string;
  sizeMb: number;
  createdAt: string;
}

export interface SnapshotMetadata {
  vmId: string;
  agentToken: string | null;
  createdAt: string;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function dirSizeMb(dir: string): number {
  let total = 0;
  for (const file of readdirSync(dir)) {
    try {
      total += statSync(join(dir, file)).size;
    } catch {
      // skip files that vanish between readdir and stat
    }
  }
  return Math.round((total / (1024 * 1024)) * 10) / 10;
}

// ---------------------------------------------------------------------------
// SnapshotService
// ---------------------------------------------------------------------------

export class SnapshotService {
  readonly paths: VmsanContext["paths"];
  readonly store: VmsanContext["store"];
  readonly logger: VmsanContext["logger"];

  constructor(ctx: VmsanContext) {
    this.paths = ctx.paths;
    this.store = ctx.store;
    this.logger = ctx.logger;
  }

  // -----------------------------------------------------------------------
  // create
  // -----------------------------------------------------------------------

  async create(opts: CreateSnapshotOptions): Promise<CreateSnapshotResult> {
    const { vmId } = opts;
    const log = this.logger.withTag(vmId);

    // Validate VM exists and is running
    const state = this.store.load(vmId);
    if (!state) throw vmNotFoundError(vmId);
    if (state.status !== "running") throw vmNotRunningError(vmId, state.status);

    const timestamp = Date.now();
    const snapshotId = `${vmId}-${timestamp}`;
    const destDir = join(this.paths.snapshotsDir, snapshotId);

    try {
      log.start("Creating snapshot via gateway...");

      // Delegate to gateway: pause → snapshot → copy → chown → resume
      const gateway = new GatewayClient();
      const result = await gateway.snapshotCreate({
        vmId,
        snapshotId,
        socketPath: state.apiSocket,
        destDir,
        chrootDir: state.chrootDir || undefined,
      });

      if (!result.ok) {
        throw new Error(result.error ?? "Gateway snapshot.create failed");
      }

      log.success("Snapshot files saved");

      // Save metadata (agent token needed for restore) — TS owns metadata
      mkdirSecure(destDir);
      const metadata: SnapshotMetadata = {
        vmId,
        agentToken: state.agentToken,
        createdAt: new Date().toISOString(),
      };
      writeSecure(join(destDir, "metadata.json"), JSON.stringify(metadata, null, 2));

      const dstSnapshotFile = join(destDir, "snapshot_file");
      const dstMemFile = join(destDir, "mem_file");

      log.success(`Snapshot ${snapshotId} created`);

      return {
        snapshotId,
        vmId,
        snapshotPath: dstSnapshotFile,
        memPath: dstMemFile,
      };
    } catch (error) {
      // Clean up partial snapshot dir
      try {
        rmSync(destDir, { recursive: true, force: true });
      } catch {
        // best-effort cleanup
      }

      throw snapshotCreateFailedError(vmId, toError(error).message);
    }
  }

  // -----------------------------------------------------------------------
  // loadMetadata
  // -----------------------------------------------------------------------

  static loadMetadata(snapshotsDir: string, snapshotId: string): SnapshotMetadata | null {
    const metaPath = join(snapshotsDir, snapshotId, "metadata.json");
    try {
      return JSON.parse(readFileSync(metaPath, "utf-8")) as SnapshotMetadata;
    } catch {
      return null;
    }
  }

  // -----------------------------------------------------------------------
  // list
  // -----------------------------------------------------------------------

  list(): SnapshotEntry[] {
    const dir = this.paths.snapshotsDir;
    if (!existsSync(dir)) return [];

    const entries: SnapshotEntry[] = [];
    for (const name of readdirSync(dir)) {
      const snapshotDir = join(dir, name);
      try {
        const stat = statSync(snapshotDir);
        if (!stat.isDirectory()) continue;

        entries.push({
          id: name,
          sizeMb: dirSizeMb(snapshotDir),
          createdAt: stat.mtime.toISOString(),
        });
      } catch {
        // skip entries that vanish
      }
    }

    return entries.sort(
      (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
    );
  }

  // -----------------------------------------------------------------------
  // delete
  // -----------------------------------------------------------------------

  delete(snapshotId: string): void {
    const snapshotDir = join(this.paths.snapshotsDir, snapshotId);
    if (!existsSync(snapshotDir)) {
      throw snapshotNotFoundError(snapshotId);
    }
    rmSync(snapshotDir, { recursive: true, force: true });
  }
}
