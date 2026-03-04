import type { ErrorOptions } from "evlog";
import type { VmErrorCode } from "./codes.ts";
import { VmsanError } from "./base.ts";

export class VmError extends VmsanError {
  readonly vmId?: string;

  constructor(code: VmErrorCode, options: ErrorOptions & { vmId?: string }) {
    super(code, options);
    this.name = "VmError";
    this.vmId = options.vmId;
  }

  override toJSON(): Record<string, unknown> {
    return { ...super.toJSON(), ...(this.vmId !== undefined && { vmId: this.vmId }) };
  }
}

export const vmNotFoundError = (vmId: string): VmError =>
  new VmError("ERR_VM_NOT_FOUND", {
    vmId,
    message: `VM not found: ${vmId}`,
    fix: "Run 'vmsan list' to see available VMs.",
  });

export const vmStateNotFoundError = (vmId: string): VmError =>
  new VmError("ERR_VM_STATE_NOT_FOUND", {
    vmId,
    message: `VM state not found: ${vmId}`,
  });

export const vmNotStoppedError = (vmId: string, currentStatus: string): VmError =>
  new VmError("ERR_VM_NOT_STOPPED", {
    vmId,
    message: `VM ${vmId} is not stopped (current status: ${currentStatus})`,
    fix: "Run 'vmsan stop <vm-id>' first, or use --force (-f) to stop and remove in one step.",
  });

export const chrootNotFoundError = (vmId: string): VmError =>
  new VmError("ERR_VM_CHROOT_NOT_FOUND", {
    vmId,
    message: `Chroot directory not found for VM ${vmId}`,
    why: "The VM data may have been removed.",
    fix: "The VM must be recreated with 'vmsan create'.",
  });

export const networkSlotsExhaustedError = (): VmError =>
  new VmError("ERR_VM_NETWORK_SLOTS_EXHAUSTED", {
    message: "No available network slots (max 255 VMs)",
  });

export const vmNotRunningError = (vmId: string, currentStatus?: string): VmError =>
  new VmError("ERR_VM_NOT_RUNNING", {
    vmId,
    message: currentStatus
      ? `VM ${vmId} is not running (current status: ${currentStatus})`
      : `VM ${vmId} is not running`,
    fix: "The VM must be running. Start it with 'vmsan start <vm-id>'.",
  });

export const vmNoAgentTokenError = (vmId: string): VmError =>
  new VmError("ERR_VM_NO_AGENT_TOKEN", {
    vmId,
    message: `VM ${vmId} has no agent token`,
    why: "This VM was created with --from-image (custom image mode) or the vmsan-agent binary was not found during creation.",
    fix: "Custom image VMs do not support connect, exec, or cp. Use port forwarding to access your service.",
  });

export const snapshotNotFoundError = (snapshotId: string): VmError =>
  new VmError("ERR_VM_SNAPSHOT_NOT_FOUND", {
    message: `Snapshot not found: ${snapshotId}`,
  });
