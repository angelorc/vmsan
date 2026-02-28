// Package entry point — re-exports public API.
export { vmsanPaths } from "./paths.ts";
export type { VmsanPaths } from "./paths.ts";

export { FirecrackerClient, firecrackerFetch } from "./services/firecracker.ts";
export type {
  paths as FirecrackerPaths,
  components as FirecrackerComponents,
} from "./generated/firecracker-api.d.ts";
export { AgentClient } from "./services/agent.ts";
export type { RunParams, RunEvent, WriteFileEntry, SessionInfo } from "./services/agent.ts";
export { VMService } from "./services/vm.ts";
export type { StopResult } from "./services/vm.ts";

export { FileVmStateStore } from "./lib/vm-state.ts";
export type { VmStateStore, VmState, VmNetwork } from "./lib/vm-state.ts";
export { NetworkManager } from "./lib/network.ts";
export type { NetworkConfig } from "./lib/network.ts";
export { Jailer } from "./lib/jailer.ts";
export type { JailerPaths, PrepareChrootConfig, SpawnJailerConfig } from "./lib/jailer.ts";
export { ShellSession, connectShell } from "./lib/shell/index.ts";
export type { ShellSessionOptions } from "./lib/shell/index.ts";
export { FileLock } from "./lib/file-lock.ts";
export { PidFile } from "./lib/pid-file.ts";
export {
  generateVmId,
  safeKill,
  isProcessAlive,
  parseDuration,
  timeAgo,
  timeRemaining,
  mkdirSecure,
  writeSecure,
  table,
} from "./lib/utils.ts";

export { VmsanError } from "./errors/index.ts";
export type { VmsanErrorCode, ValidationErrorCode, VmErrorCode, FirecrackerErrorCode, NetworkErrorCode, TimeoutErrorCode, SetupErrorCode } from "./errors/index.ts";
export { handleCommandError } from "./errors/index.ts";
export {
  ValidationError,
  invalidIntegerFlagError,
  invalidRuntimeError,
  invalidNetworkPolicyError,
  invalidPortError,
  portConflictError,
  invalidDomainError,
  invalidDomainPatternError,
  invalidCidrFormatError,
  invalidCidrPrefixError,
  invalidCidrOctetError,
  invalidImageRefEmptyError,
  invalidImageRefTagError,
  invalidDiskSizeFormatError,
  invalidDiskSizeRangeError,
  invalidDurationError,
  mutuallyExclusiveFlagsError,
  policyConflictError,
} from "./errors/index.ts";
export {
  VmError,
  vmNotFoundError,
  vmStateNotFoundError,
  vmNotStoppedError,
  chrootNotFoundError,
  networkSlotsExhaustedError,
  snapshotNotFoundError,
} from "./errors/index.ts";
export { FirecrackerApiError, firecrackerApiError } from "./errors/index.ts";
export { NetworkError, defaultInterfaceNotFoundError } from "./errors/index.ts";
export { TimeoutError, socketTimeoutError, lockTimeoutError, agentTimeoutError } from "./errors/index.ts";
export { SetupError, missingBinaryError, noKernelDirError, noKernelError, noRootfsDirError, noExt4RootfsError } from "./errors/index.ts";

export {
  initVmsanLogger,
  createCommandLogger,
  createScopedLogger,
  getOutputMode,
} from "./lib/logger/index.ts";
export type { OutputMode, CommandLogger } from "./lib/logger/index.ts";

// Command helpers for programmatic use
export { parseCreateInput } from "./commands/create/input.ts";
export type { CreateCommandRuntimeArgs } from "./commands/create/input.ts";
export { buildInitialVmState } from "./commands/create/state.ts";
export { buildCreateSummaryLines } from "./commands/create/summary.ts";
export type {
  ParsedCreateInput,
  CreateSummaryInput,
  InitialVmStateInput,
  CreateLifecycleState,
  Runtime,
  NetworkPolicy,
  VALID_RUNTIMES,
  VALID_NETWORK_POLICIES,
} from "./commands/create/types.ts";
export {
  validateEnvironment,
  findKernel,
  findRootfs,
  waitForSocket,
  getVmPid,
  getVmJailerPid,
} from "./commands/create/environment.ts";
export { waitForAgent } from "./commands/create/connect.ts";
export {
  killOrphanVmProcess,
  markVmAsError,
  cleanupNetwork,
  cleanupChroot,
} from "./commands/create/cleanup.ts";
export {
  parseVcpuCount,
  parseMemoryMib,
  parseRuntime,
  parseNetworkPolicy,
  parsePublishedPorts,
  parseDomains,
  parseDiskSizeGb,
  parseCidrList,
  validateCidr,
  validatePublishedPortsAvailable,
  parseImageReference,
} from "./commands/create/validation.ts";
export type { ImageReference } from "./commands/create/validation.ts";
export { resolveImageRootfs } from "./commands/create/image-rootfs.ts";

export async function getFirecrackerVersion(dir?: string): Promise<string | undefined> {
  const { vmsanPaths: paths } = await import("./paths.ts");
  const { FirecrackerClient: FC } = await import("./services/firecracker.ts");
  return FC.getVersion(dir || paths().baseDir);
}
