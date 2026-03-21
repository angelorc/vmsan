import type { CreateSummaryInput } from "./types.ts";

export function buildCreateSummaryLines(input: CreateSummaryInput): string[] {
  return [
    `VM Created: ${input.vmId}`,
    "",
    "  Status:   running",
    `  PID:      ${input.pid || "unknown"}`,
    `  vCPUs:    ${input.vcpus}`,
    `  Memory:   ${input.memMib} MiB`,
    `  Runtime:  ${input.runtime}`,
    `  Disk:     ${input.diskSizeGb} GB`,
    ...(input.project ? [`  Project:  ${input.project}`] : []),
    "",
    "  Network:",
    `    TAP:    ${input.tapDevice}`,
    `    Host:   ${input.hostIp}`,
    `    Guest:  ${input.guestIp}`,
    `    MAC:    ${input.macAddress}`,
    `    Policy: ${input.networkPolicy}`,
    ...(input.domains.length > 0 ? [`    Domains: ${input.domains.join(", ")}`] : []),
    ...(input.allowedCidrs.length > 0
      ? [`    Allowed CIDRs: ${input.allowedCidrs.join(", ")}`]
      : []),
    ...(input.deniedCidrs.length > 0 ? [`    Denied CIDRs:  ${input.deniedCidrs.join(", ")}`] : []),
    ...(input.ports.length > 0 ? [`    Ports:  ${input.ports.join(", ")}`] : []),
    ...(input.service ? [`    Service: ${input.service}`] : []),
    ...(input.connectTo?.length ? [`    Connect-to: ${input.connectTo.join(", ")}`] : []),
    ...(input.tunnelHostnames?.length
      ? input.tunnelHostnames.map((h) => `    Tunnel: https://${h}`)
      : []),
    "",
    `  Kernel:   ${input.kernelPath}`,
    `  Rootfs:   ${input.rootfsPath}`,
    ...(input.snapshotId ? [`  Snapshot: ${input.snapshotId}`] : []),
    ...(input.timeout ? [`  Timeout:  ${input.timeout}`] : []),
    "",
    `  Socket:   ${input.socketPath}`,
    `  Chroot:   ${input.chrootDir}`,
    `  State:    ${input.stateFilePath}`,
  ];
}
