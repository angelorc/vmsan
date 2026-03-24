import { existsSync, readdirSync, readFileSync, unlinkSync } from "node:fs";
import { join } from "node:path";
import consola from "consola";
import { vmStateNotFoundError, networkSlotsExhaustedError } from "../errors/index.ts";
import { slotFromVmHostIpOrNull } from "./network-address.ts";
import { mkdirSecure, writeSecure } from "./utils.ts";

export const CURRENT_STATE_VERSION = 2;

export interface VmNetwork {
  tapDevice: string;
  hostIp: string;
  guestIp: string;
  subnetMask: string;
  macAddress: string;
  networkPolicy: string;
  allowedDomains: string[];
  allowedCidrs: string[];
  deniedCidrs: string[];
  publishedPorts: number[];
  tunnelHostname: string | null;
  tunnelHostnames?: string[];
  bandwidthMbit?: number;
  netnsName?: string;
  skipDnat?: boolean;
  allowIcmp?: boolean;
  firewallBackend?: "nftables";
  connectTo?: string[];
  meshIp?: string;
  service?: string;
}

export interface VmState {
  id: string;
  project: string;
  runtime: string;
  diskSizeGb?: number;
  status: "creating" | "running" | "stopped" | "error";
  pid: number | null;
  apiSocket: string;
  chrootDir: string;
  kernel: string;
  rootfs: string;
  vcpuCount: number;
  memSizeMib: number;
  network: VmNetwork;
  snapshot: string | null;
  timeoutMs: number | null;
  timeoutAt: string | null;
  createdAt: string;
  error: string | null;
  agentToken: string | null;
  agentPort: number;
  stateVersion: number;
  disableSeccomp?: boolean;
  disablePidNs?: boolean;
  disableCgroup?: boolean;
}

export interface VmStateStore {
  save(state: VmState): void;
  load(id: string): VmState | null;
  list(): VmState[];
  update(id: string, updates: Partial<VmState>): void;
  delete(id: string): void;
  allocateNetworkSlot(): number;
}

export function findFreeNetworkSlot(states: VmState[]): number {
  // Network slot allocation is now authoritative in the gateway.
  // This function is kept for backward compat but only checks local state.
  const usedSlots = new Set<number>();
  for (const state of states) {
    if (state.status === "error") continue;
    const slot = slotFromVmHostIpOrNull(state.network.hostIp);
    if (slot !== null) {
      usedSlots.add(slot);
    }
  }

  for (let slot = 0; slot <= 254; slot++) {
    if (!usedSlots.has(slot)) return slot;
  }

  throw networkSlotsExhaustedError();
}

export class FileVmStateStore implements VmStateStore {
  constructor(private readonly dir: string) {}

  private ensureDir(): void {
    mkdirSecure(this.dir);
  }

  save(state: VmState): void {
    this.ensureDir();
    state.stateVersion = CURRENT_STATE_VERSION;
    writeSecure(join(this.dir, `${state.id}.json`), JSON.stringify(state, null, 2));
  }

  load(id: string): VmState | null {
    const filePath = join(this.dir, `${id}.json`);
    if (!existsSync(filePath)) return null;
    const state = JSON.parse(readFileSync(filePath, "utf-8")) as VmState;
    let migrated = false;
    if (!state.stateVersion) {
      consola.debug(`Migrated state file for VM ${id} from v0 to v1`);
      state.stateVersion = 1;
      migrated = true;
    }
    if (state.stateVersion === 1) {
      consola.debug(`Migrated state file for VM ${id} from v1 to v2`);
      state.disableSeccomp = state.disableSeccomp ?? false;
      state.disablePidNs = state.disablePidNs ?? false;
      state.disableCgroup = state.disableCgroup ?? false;
      state.stateVersion = 2;
      migrated = true;
    }
    if (migrated) {
      this.save(state);
    }
    return state;
  }

  list(): VmState[] {
    this.ensureDir();
    const files = readdirSync(this.dir).filter((f) => f.endsWith(".json"));
    return files.map((f) => {
      const id = f.replace(/\.json$/, "");
      return this.load(id)!;
    });
  }

  update(id: string, updates: Partial<VmState>): void {
    const state = this.load(id);
    if (!state) throw vmStateNotFoundError(id);
    Object.assign(state, updates);
    this.save(state);
  }

  delete(id: string): void {
    const filePath = join(this.dir, `${id}.json`);
    if (existsSync(filePath)) unlinkSync(filePath);
  }

  allocateNetworkSlot(): number {
    return findFreeNetworkSlot(this.list());
  }
}

/** @deprecated Gateway now owns slot allocation. Kept for backward compat. */
export function getActiveTapSlots(): Set<number> {
  // Slot allocation is now authoritative in the gateway.
  // Return empty set — the gateway is the single source of truth.
  return new Set<number>();
}
