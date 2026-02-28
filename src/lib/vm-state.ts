import { existsSync, readdirSync, readFileSync, unlinkSync } from "node:fs";
import { join } from "node:path";
import { vmStateNotFoundError, networkSlotsExhaustedError } from "../errors/index.ts";
import { mkdirSecure, writeSecure } from "./utils.ts";

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
}

export interface VmStateStore {
  save(state: VmState): void;
  load(id: string): VmState | null;
  list(): VmState[];
  update(id: string, updates: Partial<VmState>): void;
  delete(id: string): void;
  allocateNetworkSlot(): number;
}

export class FileVmStateStore implements VmStateStore {
  constructor(private readonly dir: string) {}

  private ensureDir(): void {
    mkdirSecure(this.dir);
  }

  save(state: VmState): void {
    this.ensureDir();
    writeSecure(join(this.dir, `${state.id}.json`), JSON.stringify(state, null, 2));
  }

  load(id: string): VmState | null {
    const filePath = join(this.dir, `${id}.json`);
    if (!existsSync(filePath)) return null;
    return JSON.parse(readFileSync(filePath, "utf-8")) as VmState;
  }

  list(): VmState[] {
    this.ensureDir();
    const files = readdirSync(this.dir).filter((f) => f.endsWith(".json"));
    return files.map((f) => JSON.parse(readFileSync(join(this.dir, f), "utf-8")) as VmState);
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
    const states = this.list();
    const usedSlots = new Set(
      states
        .filter((s) => s.status === "running" || s.status === "creating")
        .map((s) => {
          const parts = s.network.hostIp.split(".");
          return Number(parts[2]);
        }),
    );
    for (const slot of FileVmStateStore.getActiveTapSlots()) {
      usedSlots.add(slot);
    }

    for (let slot = 0; slot <= 254; slot++) {
      if (!usedSlots.has(slot)) return slot;
    }

    throw networkSlotsExhaustedError();
  }

  private static getActiveTapSlots(): Set<number> {
    const slots = new Set<number>();
    try {
      for (const iface of readdirSync("/sys/class/net")) {
        // Match TAP devices (fhvmN) and veth host ends (veth-h-N)
        const tapMatch = /^fhvm(\d+)$/.exec(iface);
        const vethMatch = /^veth-h-(\d+)$/.exec(iface);
        const match = tapMatch || vethMatch;
        if (!match) continue;
        const slot = Number(match[1]);
        if (Number.isInteger(slot) && slot >= 0 && slot <= 254) {
          slots.add(slot);
        }
      }
    } catch {
      // /sys/class/net may not be readable on some systems
    }
    return slots;
  }
}
