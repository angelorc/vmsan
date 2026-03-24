// Package entry point — re-exports public API.

// Building blocks
export { createVmsan } from "./context.ts";
export type { VmsanContext, VmsanOptions } from "./context.ts";
export type { VmsanHooks, VmPhase } from "./hooks.ts";
export { definePlugin } from "./plugin.ts";
export type { VmsanPlugin } from "./plugin.ts";
export type { VmsanLogger } from "./vmsan-logger.ts";
export { createDefaultLogger } from "./vmsan-logger.ts";

export { vmsanPaths } from "./paths.ts";
export type { VmsanPaths } from "./paths.ts";

export { AgentClient } from "./services/agent.ts";
export type { RunParams, RunEvent, WriteFileEntry } from "./services/agent.ts";
export { resolveVmState } from "./lib/vm-context.ts";
export type { RunningVmContext } from "./lib/vm-context.ts";
export { VMService } from "./services/vm.ts";
export type {
  StopResult,
  UpdatePolicyResult,
  CreateVmOptions,
  CreateVmResult,
  StartVmResult,
} from "./services/vm.ts";

export { FileVmStateStore } from "./lib/vm-state.ts";
export type { VmStateStore, VmState, VmNetwork } from "./lib/vm-state.ts";
export { MemoryVmStateStore } from "./stores/memory.ts";
export { SqliteVmStateStore, createStateStore } from "./lib/state/index.ts";
export type { HostState, SyncLogEntry } from "./lib/state/index.ts";
export { GatewayClient, ensureGatewayRunning } from "./lib/gateway-client.ts";
export type {
  GatewayVmMetadata,
  GatewayVmNetworkMeta,
  GatewayStatusResult,
  GatewayVmGetResult,
  GatewayDoctorCheck,
  GatewayDoctorResult,
  GatewayRootfsDownloadParams,
  GatewayRootfsDownloadResult,
  GatewayCfSetupParams,
  GatewayCfAddRouteParams,
  GatewayCfRemoveRouteParams,
  GatewayCfStatusResult,
  GatewayExtendTimeoutParams,
} from "./lib/gateway-client.ts";
export { ShellSession, connectShell } from "./lib/shell/index.ts";
export type { ShellSessionOptions } from "./lib/shell/index.ts";
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
  Runtime,
  NetworkPolicy,
  VALID_RUNTIMES,
  VALID_NETWORK_POLICIES,
} from "./commands/create/types.ts";
export {
  validateEnvironment,
  findKernel,
  findRootfs,
  findRuntimeRootfs,
} from "./commands/create/environment.ts";
export { waitForAgent } from "./lib/vm-context.ts";
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
  parseBandwidth,
  parseConnectTo,
} from "./commands/create/validation.ts";
export type { ImageReference } from "./commands/create/validation.ts";
export { resolveImageRootfs } from "./commands/create/image-rootfs.ts";

// Secrets
export { SecretsStore } from "./lib/secrets/index.ts";

// Cloudflare plugin
export { cloudflarePlugin } from "./plugins/cloudflare.ts";
export { CloudflareService, resolveTunnelHostnames } from "./services/cloudflare.ts";
export type { CloudflareConfig, TunnelRoute } from "./services/cloudflare.ts";
export { CloudflareError } from "./errors/cloudflare.ts";
export { cleanupCloudflareResources } from "./lib/cloudflare-cleanup.ts";
