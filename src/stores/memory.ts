import { getActiveTapSlots, type VmState, type VmStateStore } from "../lib/vm-state.ts";
import { vmStateNotFoundError, networkSlotsExhaustedError } from "../errors/index.ts";

export class MemoryVmStateStore implements VmStateStore {
  private states = new Map<string, VmState>();

  save(state: VmState): void {
    this.states.set(state.id, structuredClone(state));
  }

  load(id: string): VmState | null {
    const state = this.states.get(id);
    return state ? structuredClone(state) : null;
  }

  list(): VmState[] {
    return [...this.states.values()].map((s) => structuredClone(s));
  }

  update(id: string, updates: Partial<VmState>): void {
    const state = this.states.get(id);
    if (!state) throw vmStateNotFoundError(id);
    Object.assign(state, updates);
  }

  delete(id: string): void {
    this.states.delete(id);
  }

  allocateNetworkSlot(): number {
    const usedSlots = new Set(
      [...this.states.values()]
        .filter((s) => s.status === "running" || s.status === "creating")
        .map((s) => {
          const parts = s.network.hostIp.split(".");
          return Number(parts[2]);
        }),
    );
    for (const slot of getActiveTapSlots()) {
      usedSlots.add(slot);
    }

    for (let slot = 0; slot <= 254; slot++) {
      if (!usedSlots.has(slot)) return slot;
    }

    throw networkSlotsExhaustedError();
  }
}
