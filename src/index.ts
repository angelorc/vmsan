// Package entry point — re-exports public SDK API.

// Building blocks
export { createVmsan, createVmsanContext } from "./context.ts";
export type { VmsanContext, VmsanOptions } from "./context.ts";
export type { VmsanHooks, VmPhase, CreateVmOptions, NetworkPolicy, Runtime, ImageReference } from "./hooks.ts";
export { definePlugin } from "./plugin.ts";
export type { VmsanPlugin } from "./plugin.ts";
export type { VmsanLogger } from "./vmsan-logger.ts";
export { createDefaultLogger } from "./vmsan-logger.ts";

export { vmsanPaths } from "./paths.ts";
export type { VmsanPaths } from "./paths.ts";

export { AgentClient } from "./services/agent.ts";
export type { RunParams, RunEvent, WriteFileEntry } from "./services/agent.ts";
export { Command, CommandFinished } from "./lib/command.ts";
export type { CommandInit, LogEntry } from "./lib/command.ts";
export { resolveVmState, waitForAgent } from "./lib/vm-context.ts";
export type { RunningVmContext } from "./lib/vm-context.ts";
export { SnapshotService } from "./services/snapshot.ts";
export type {
  CreateSnapshotOptions,
  CreateSnapshotResult,
  SnapshotEntry,
  SnapshotMetadata,
} from "./services/snapshot.ts";

export { FileVmStateStore } from "./lib/vm-state.ts";
export type { VmStateStore, VmState, VmNetwork } from "./lib/vm-state.ts";
export { MemoryVmStateStore } from "./stores/memory.ts";
export { SqliteVmStateStore, createStateStore } from "./lib/state/index.ts";
export type { HostState, SyncLogEntry } from "./lib/state/index.ts";
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
  toError,
} from "./lib/utils.ts";

export { VmsanError } from "./errors/index.ts";
export type {
  VmsanErrorCode,
  ValidationErrorCode,
  VmErrorCode,
  NetworkErrorCode,
  TimeoutErrorCode,
  SetupErrorCode,
  CloudflareErrorCode,
} from "./errors/index.ts";
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
  vmNotRunningError,
  vmNoAgentTokenError,
  networkSlotsExhaustedError,
  snapshotNotFoundError,
} from "./errors/index.ts";
export { NetworkError, defaultInterfaceNotFoundError } from "./errors/index.ts";
export {
  TimeoutError,
  socketTimeoutError,
  lockTimeoutError,
  agentTimeoutError,
} from "./errors/index.ts";
export {
  SetupError,
  missingBinaryError,
  noKernelDirError,
  noKernelError,
  noRootfsDirError,
  noExt4RootfsError,
} from "./errors/index.ts";

// Secrets
export { SecretsStore } from "./lib/secrets/index.ts";

// Cloudflare plugin
export { cloudflarePlugin } from "./plugins/cloudflare.ts";
export { CloudflareService, resolveTunnelHostnames } from "./services/cloudflare.ts";
export type { CloudflareConfig, TunnelRoute } from "./services/cloudflare.ts";
export { CloudflareError } from "./errors/cloudflare.ts";
export { cleanupCloudflareResources } from "./lib/cloudflare-cleanup.ts";

// Network utilities
export { slotFromVmHostIp } from "./lib/network-address.ts";

// Rootfs manager
export type { RootfsType } from "./lib/rootfs-manager.ts";
export { downloadRootfs, getRootfsPath, verifyChecksum, getCacheDir } from "./lib/rootfs-manager.ts";

// PID file utilities
export { PidFile } from "./lib/pid-file.ts";
