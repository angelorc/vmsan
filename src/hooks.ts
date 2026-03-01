import type { VmState } from "./lib/vm-state.ts";
import type { NetworkConfig } from "./lib/network.ts";

export interface VmsanHooks {
  // VM lifecycle
  "vm:beforeCreate": (params: {
    vmId: string;
    options: Record<string, unknown>;
  }) => void | Promise<void>;
  "vm:afterCreate": (state: VmState) => void | Promise<void>;
  "vm:beforeStart": (params: { vmId: string; state: VmState }) => void | Promise<void>;
  "vm:afterStart": (state: VmState) => void | Promise<void>;
  "vm:beforeStop": (params: { vmId: string; state: VmState }) => void | Promise<void>;
  "vm:afterStop": (params: { vmId: string; previousStatus: string }) => void | Promise<void>;
  "vm:beforeRemove": (params: {
    vmId: string;
    state: VmState;
    force: boolean;
  }) => void | Promise<void>;
  "vm:afterRemove": (params: { vmId: string }) => void | Promise<void>;
  "vm:error": (params: { vmId: string; error: Error; phase: string }) => void | Promise<void>;

  // Network
  "network:afterSetup": (params: {
    vmId: string;
    slot: number;
    networkConfig: NetworkConfig;
    domains: string[];
    networkPolicy: string;
  }) => void | Promise<void>;
  "network:afterTeardown": (params: {
    vmId: string;
    networkConfig: NetworkConfig;
  }) => void | Promise<void>;
  "network:policyChange": (params: {
    vmId: string;
    previousPolicy: string;
    newPolicy: string;
  }) => void | Promise<void>;

  // State changes
  "state:change": (params: {
    vmId: string;
    field: string;
    oldValue: unknown;
    newValue: unknown;
  }) => void | Promise<void>;
}
