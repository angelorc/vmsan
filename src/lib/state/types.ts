export interface HostState {
  id: string;
  name: string;
  address: string;
  publicKey?: string;
  status: "pending" | "active" | "draining" | "offline";
  resources?: { cpus: number; memoryMB: number; diskGB: number };
  lastHeartbeat?: string;
  createdAt: string;
}

export interface SyncLogEntry {
  version?: number; // auto-assigned on insert
  entityType: "vm" | "host" | "project" | "mesh";
  entityId: string;
  operation: "create" | "update" | "delete";
  payload?: unknown;
  createdAt?: string;
}
