import {
  copyFileSync,
  chmodSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
} from "node:fs";
import { join } from "node:path";
import type { VmsanContext } from "../context.ts";
import { FirecrackerClient } from "./firecracker.ts";
import {
  vmNotFoundError,
  vmNotRunningError,
  snapshotNotFoundError,
  snapshotCreateFailedError,
} from "../errors/index.ts";
import { mkdirSecure, writeSecure, chownToSudoUser, toError } from "../lib/utils.ts";

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

function copyFileSecure(src: string, dst: string): void {
  copyFileSync(src, dst);
  chmodSync(dst, 0o600);
  chownToSudoUser(dst);
}

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
    const { vmId, resume = true } = opts;
    const log = this.logger.withTag(vmId);

    // Validate VM exists and is running
    const state = this.store.load(vmId);
    if (!state) throw vmNotFoundError(vmId);
    if (state.status !== "running") throw vmNotRunningError(vmId, state.status);

    const fc = new FirecrackerClient(state.apiSocket);
    const timestamp = Date.now();
    const snapshotId = `${vmId}-${timestamp}`;

    // Paths inside the chroot (relative to jailer root)
    const chrootSnapshotFile = "snapshot/snapshot_file";
    const chrootMemFile = "snapshot/mem_file";

    // Absolute path to the chroot's snapshot dir (chrootDir/root/snapshot)
    const chrootSnapshotDir = join(state.chrootDir, "root", "snapshot");

    // Destination in ~/.vmsan/snapshots/<snapshotId>/
    const destDir = join(this.paths.snapshotsDir, snapshotId);

    try {
      // 1. Pause VM
      log.start("Pausing VM...");
      await fc.pause();
      log.success("VM paused");

      // 2. Ensure chroot snapshot directory exists
      mkdirSync(chrootSnapshotDir, { recursive: true });

      // 3. Create snapshot via Firecracker API
      log.start("Creating snapshot...");
      await fc.createSnapshot(chrootSnapshotFile, chrootMemFile);
      log.success("Snapshot created in chroot");

      // 4. Copy files to persistent storage
      log.start("Copying snapshot files...");
      mkdirSecure(destDir);

      const srcSnapshotFile = join(chrootSnapshotDir, "snapshot_file");
      const srcMemFile = join(chrootSnapshotDir, "mem_file");
      const dstSnapshotFile = join(destDir, "snapshot_file");
      const dstMemFile = join(destDir, "mem_file");

      copyFileSecure(srcSnapshotFile, dstSnapshotFile);
      copyFileSecure(srcMemFile, dstMemFile);

      // 5. Save metadata (agent token needed for restore)
      const metadata: SnapshotMetadata = {
        vmId,
        agentToken: state.agentToken,
        createdAt: new Date().toISOString(),
      };
      writeSecure(join(destDir, "metadata.json"), JSON.stringify(metadata, null, 2));

      log.success(`Snapshot files saved to ${destDir}`);

      // 6. Resume VM (unless --no-resume)
      if (resume) {
        try {
          await fc.resume();
          log.success("VM resumed");
        } catch (err) {
          log.warn(`Failed to resume VM: ${toError(err).message}`);
        }
      }

      return {
        snapshotId,
        vmId,
        snapshotPath: dstSnapshotFile,
        memPath: dstMemFile,
      };
    } catch (error) {
      // Always try to resume on failure
      if (resume) {
        try {
          await fc.resume();
        } catch (resumeErr) {
          this.logger.debug(`Resume after error failed: ${toError(resumeErr).message}`);
        }
      }

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
