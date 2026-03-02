import { findFreeNetworkSlot, type VmState, type VmStateStore } from "../lib/vm-state.ts";
import { vmStateNotFoundError } from "../errors/index.ts";

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
    return findFreeNetworkSlot([...this.states.values()]);
  }
}
