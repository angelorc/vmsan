import type { VmState } from "../../lib/vm-state.ts";
import type { InitialVmStateInput } from "./types.ts";

export function buildInitialVmState(input: InitialVmStateInput): VmState {
  const now = new Date().toISOString();

  return {
    id: input.vmId,
    project: input.project,
    runtime: input.runtime,
    diskSizeGb: input.diskSizeGb,
    status: "creating",
    pid: null,
    apiSocket: "",
    chrootDir: "",
    kernel: input.kernelPath,
    rootfs: input.rootfsPath,
    vcpuCount: input.vcpus,
    memSizeMib: input.memMib,
    network: {
      tapDevice: input.tapDevice,
      hostIp: input.hostIp,
      guestIp: input.guestIp,
      subnetMask: input.subnetMask,
      macAddress: input.macAddress,
      networkPolicy: input.networkPolicy,
      allowedDomains: input.domains,
      allowedCidrs: input.allowedCidrs,
      deniedCidrs: input.deniedCidrs,
      publishedPorts: input.ports,
      tunnelHostname: null,
      tunnelHostnames: [],
      bandwidthMbit: input.bandwidthMbit,
      netnsName: input.netnsName,
    },
    snapshot: input.snapshotId,
    timeoutMs: input.timeoutMs,
    timeoutAt: input.timeoutMs ? new Date(Date.now() + input.timeoutMs).toISOString() : null,
    createdAt: now,
    error: null,
    agentToken: input.agentToken,
    agentPort: input.agentPort,
  };
}
