import type { VmState } from "../../lib/vm-state.ts";

export const VALID_RUNTIMES = ["base", "node22", "node24", "python3.13"] as const;
export type Runtime = (typeof VALID_RUNTIMES)[number];

export const VALID_NETWORK_POLICIES = ["allow-all", "deny-all", "custom"] as const;
export type NetworkPolicy = (typeof VALID_NETWORK_POLICIES)[number];

export interface ParsedCreateInput {
  vcpus: number;
  memMib: number;
  runtime: Runtime;
  networkPolicy: NetworkPolicy;
  ports: number[];
  domains: string[];
  allowedCidrs: string[];
  deniedCidrs: string[];
  timeoutMs: number | null;
  snapshotId: string | null;
  diskSizeGb: number;
}

export interface CreateSummaryInput {
  vmId: string;
  pid: number | null;
  vcpus: number;
  memMib: number;
  runtime: Runtime;
  diskSizeGb: number;
  project: string;
  networkPolicy: NetworkPolicy;
  domains: string[];
  allowedCidrs: string[];
  deniedCidrs: string[];
  ports: number[];
  kernelPath: string;
  rootfsPath: string;
  snapshotId: string | null;
  timeout: string | undefined;
  socketPath: string;
  chrootDir: string;
  tapDevice: string;
  hostIp: string;
  guestIp: string;
  macAddress: string;
  stateFilePath: string;
  tunnelHostnames?: string[];
  connectTo?: string[];
  service?: string;
}

export interface InitialVmStateInput {
  vmId: string;
  project: string;
  runtime: Runtime;
  diskSizeGb: number;
  kernelPath: string;
  rootfsPath: string;
  vcpus: number;
  memMib: number;
  networkPolicy: NetworkPolicy;
  domains: string[];
  allowedCidrs: string[];
  deniedCidrs: string[];
  ports: number[];
  tapDevice: string;
  hostIp: string;
  guestIp: string;
  subnetMask: string;
  macAddress: string;
  snapshotId: string | null;
  timeoutMs: number | null;
  agentToken: string | null;
  agentPort: number;
  bandwidthMbit?: number;
  netnsName?: string;
  skipDnat?: boolean;
  allowIcmp?: boolean;
  disableSeccomp?: boolean;
  disablePidNs?: boolean;
  disableCgroup?: boolean;
  connectTo?: string[];
  service?: string;
}

export type CreateState = VmState;
