import { dirname } from "node:path";
import consola from "consola";
import {
  CURRENT_STATE_VERSION,
  findFreeNetworkSlot,
  type VmState,
  type VmStateStore,
} from "../vm-state.ts";
import { vmStateNotFoundError } from "../../errors/index.ts";
import { mkdirSecure } from "../utils.ts";
import type { HostState, SyncLogEntry } from "./types.ts";

// Runtime-adaptive SQLite: use bun:sqlite under Bun, better-sqlite3 under Node.js
interface SqliteDb {
  exec(sql: string): void;
  prepare(sql: string): { run(...params: unknown[]): unknown; get(...params: unknown[]): unknown; all(...params: unknown[]): unknown[] };
  close(): void;
}

function openDatabase(dbPath: string): SqliteDb {
  // @ts-expect-error — Bun global detection
  if (typeof Bun !== "undefined") {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const { Database } = require("bun:sqlite");
    return new Database(dbPath) as SqliteDb;
  }
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const BetterSqlite3 = require("better-sqlite3");
  return new BetterSqlite3(dbPath) as SqliteDb;
}

const SCHEMA = `
CREATE TABLE IF NOT EXISTS vms (
    id TEXT PRIMARY KEY,
    name TEXT,
    project TEXT,
    service TEXT,
    host_id TEXT,
    state_json TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'stopped',
    deploy_hash TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS hosts (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    address TEXT NOT NULL,
    public_key TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    resources_json TEXT,
    last_heartbeat DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    config_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS mesh_allocations (
    vm_id TEXT PRIMARY KEY,
    mesh_ip TEXT UNIQUE NOT NULL,
    project TEXT NOT NULL,
    service TEXT
);

CREATE TABLE IF NOT EXISTS sync_log (
    version INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    payload_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_vms_project ON vms(project);
CREATE INDEX IF NOT EXISTS idx_vms_host ON vms(host_id);
CREATE INDEX IF NOT EXISTS idx_vms_status ON vms(status);
CREATE INDEX IF NOT EXISTS idx_sync_log_version ON sync_log(version);
CREATE INDEX IF NOT EXISTS idx_mesh_project ON mesh_allocations(project);
`;

export class SqliteVmStateStore implements VmStateStore {
  private db: SqliteDb;

  constructor(dbPath: string) {
    mkdirSecure(dirname(dbPath));
    this.db = openDatabase(dbPath);
    this.db.exec("PRAGMA journal_mode=WAL");
    this.db.exec("PRAGMA busy_timeout=5000");
    this.initSchema();
  }

  private initSchema(): void {
    this.db.exec(SCHEMA);
  }

  save(state: VmState): void {
    state.stateVersion = CURRENT_STATE_VERSION;
    const stateJson = JSON.stringify(state);
    const service = state.network?.service ?? null;

    this.db
      .prepare(
        `INSERT OR REPLACE INTO vms (id, name, project, service, state_json, status, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
      )
      .run(state.id, state.id, state.project, service, stateJson, state.status, state.createdAt);

    // Sync mesh allocation if present
    if (state.network?.meshIp) {
      this.db
        .prepare(
          `INSERT OR REPLACE INTO mesh_allocations (vm_id, mesh_ip, project, service)
           VALUES (?, ?, ?, ?)`,
        )
        .run(state.id, state.network.meshIp, state.project, service);
    }
  }

  load(id: string): VmState | null {
    const row = this.db.prepare("SELECT state_json FROM vms WHERE id = ?").get(id) as
      | { state_json: string }
      | null;
    if (!row) return null;
    return JSON.parse(row.state_json) as VmState;
  }

  list(): VmState[] {
    const rows = this.db.prepare("SELECT state_json FROM vms").all() as { state_json: string }[];
    return rows.map((row) => JSON.parse(row.state_json) as VmState);
  }

  update(id: string, updates: Partial<VmState>): void {
    const state = this.load(id);
    if (!state) throw vmStateNotFoundError(id);
    Object.assign(state, updates);
    this.save(state);
  }

  delete(id: string): void {
    this.db.prepare("DELETE FROM mesh_allocations WHERE vm_id = ?").run(id);
    this.db.prepare("DELETE FROM vms WHERE id = ?").run(id);
  }

  allocateNetworkSlot(): number {
    return findFreeNetworkSlot(this.list());
  }

  // --- Extended methods for 0.8.0 ---

  listByProject(project: string): VmState[] {
    const rows = this.db
      .prepare("SELECT state_json FROM vms WHERE project = ?")
      .all(project) as { state_json: string }[];
    return rows.map((row) => JSON.parse(row.state_json) as VmState);
  }

  // --- Host operations ---

  saveHost(host: HostState): void {
    const resourcesJson = host.resources ? JSON.stringify(host.resources) : null;
    this.db
      .prepare(
        `INSERT OR REPLACE INTO hosts (id, name, address, public_key, status, resources_json, last_heartbeat, created_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
      )
      .run(
        host.id,
        host.name,
        host.address,
        host.publicKey ?? null,
        host.status,
        resourcesJson,
        host.lastHeartbeat ?? null,
        host.createdAt,
      );
  }

  loadHost(id: string): HostState | null {
    const row = this.db.prepare("SELECT * FROM hosts WHERE id = ?").get(id) as Record<
      string,
      unknown
    > | null;
    if (!row) return null;
    return rowToHostState(row);
  }

  listHosts(): HostState[] {
    const rows = this.db.prepare("SELECT * FROM hosts").all() as Record<string, unknown>[];
    return rows.map(rowToHostState);
  }

  deleteHost(id: string): void {
    this.db.prepare("DELETE FROM hosts WHERE id = ?").run(id);
  }

  // --- Sync log ---

  appendSyncLog(entry: SyncLogEntry): void {
    const payloadJson = entry.payload !== undefined ? JSON.stringify(entry.payload) : null;
    this.db
      .prepare(
        `INSERT INTO sync_log (entity_type, entity_id, operation, payload_json)
         VALUES (?, ?, ?, ?)`,
      )
      .run(entry.entityType, entry.entityId, entry.operation, payloadJson);
  }

  readSyncLogSince(version: number): SyncLogEntry[] {
    const rows = this.db
      .prepare("SELECT * FROM sync_log WHERE version > ? ORDER BY version ASC")
      .all(version) as Record<string, unknown>[];
    return rows.map(rowToSyncLogEntry);
  }

  vmCount(): number {
    const row = this.db.prepare("SELECT COUNT(*) as count FROM vms").get() as { count: number };
    return row.count;
  }

  close(): void {
    try {
      this.db.close();
    } catch (error) {
      consola.debug(`Failed to close SQLite database: ${(error as Error).message}`);
    }
  }
}

function rowToHostState(row: Record<string, unknown>): HostState {
  return {
    id: row.id as string,
    name: row.name as string,
    address: row.address as string,
    publicKey: (row.public_key as string) ?? undefined,
    status: row.status as HostState["status"],
    resources: row.resources_json ? JSON.parse(row.resources_json as string) : undefined,
    lastHeartbeat: (row.last_heartbeat as string) ?? undefined,
    createdAt: row.created_at as string,
  };
}

function rowToSyncLogEntry(row: Record<string, unknown>): SyncLogEntry {
  return {
    version: row.version as number,
    entityType: row.entity_type as SyncLogEntry["entityType"],
    entityId: row.entity_id as string,
    operation: row.operation as SyncLogEntry["operation"],
    payload: row.payload_json ? JSON.parse(row.payload_json as string) : undefined,
    createdAt: (row.created_at as string) ?? undefined,
  };
}
