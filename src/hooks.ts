import type { VmState } from "./lib/vm-state.ts";

export type NetworkPolicy = "allow-all" | "deny-all" | "custom";
export type Runtime = "base" | "node22" | "node24" | "python3.13";

export interface ImageReference {
  full: string;
  name: string;
  tag: string;
  cacheKey: string;
}

export interface CreateVmOptions {
  vcpus?: number;
  memMib?: number;
  diskSizeGb?: number;
  kernelPath?: string;
  rootfsPath?: string;
  fromImage?: ImageReference;
  project?: string;
  runtime?: Runtime;
  networkPolicy?: NetworkPolicy;
  domains?: string[];
  allowedCidrs?: string[];
  deniedCidrs?: string[];
  ports?: number[];
  bandwidthMbit?: number;
  allowIcmp?: boolean;
  disableNetns?: boolean;
  disableSeccomp?: boolean;
  disablePidNs?: boolean;
  disableCgroup?: boolean;
  timeoutMs?: number;
  snapshotId?: string;
  skipDnat?: boolean;
  connectTo?: string[];
  service?: string;
}

export type VmPhase = "create" | "start" | "stop" | "remove";

export interface VmsanHooks {
  // VM lifecycle
  "vm:beforeCreate": (params: { vmId: string; options: CreateVmOptions }) => void | Promise<void>;
  "vm:afterCreate": (state: VmState) => void | Promise<void>;
  "vm:beforeStart": (params: { vmId: string; state: VmState }) => void | Promise<void>;
  "vm:afterStart": (state: VmState) => void | Promise<void>;
  "vm:beforeStop": (params: { vmId: string; state: VmState }) => void | Promise<void>;
  "vm:afterStop": (params: {
    vmId: string;
    previousStatus: VmState["status"];
  }) => void | Promise<void>;
  "vm:beforeRemove": (params: {
    vmId: string;
    state: VmState;
    force: boolean;
  }) => void | Promise<void>;
  "vm:afterRemove": (params: { vmId: string }) => void | Promise<void>;
  "vm:error": (params: { vmId: string; error: Error; phase: VmPhase }) => void | Promise<void>;

  // Network
  "network:policyChange": (params: {
    vmId: string;
    previousPolicy: NetworkPolicy;
    newPolicy: NetworkPolicy;
  }) => void | Promise<void>;
}
