import { describe, expect, it, beforeEach } from "vitest";
import { createLogger, initLogger } from "evlog";
import { VmsanError } from "../../src/errors/base.ts";
import { ValidationError } from "../../src/errors/validation.ts";
import { VmError } from "../../src/errors/vm.ts";
import { TimeoutError } from "../../src/errors/timeout.ts";
import { FirecrackerApiError } from "../../src/errors/firecracker.ts";
import { CloudflareError } from "../../src/errors/cloudflare.ts";
import { handleCommandError } from "../../src/errors/display.ts";
import type { CommandLogger } from "../../src/lib/logger/index.ts";
import {
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
  vmNotFoundError,
  vmStateNotFoundError,
  vmNotStoppedError,
  vmNotRunningError,
  vmNoAgentTokenError,
  chrootNotFoundError,
  networkSlotsExhaustedError,
  snapshotNotFoundError,
  firecrackerApiError,
  defaultInterfaceNotFoundError,
  socketTimeoutError,
  lockTimeoutError,
  agentTimeoutError,
  missingBinaryError,
  noKernelDirError,
  noKernelError,
  noRootfsDirError,
  noExt4RootfsError,
  cloudflareNotConfiguredError,
  cloudflareTunnelNoIdError,
  cloudflaredNotFoundError,
  cloudflareConfigNotFoundError,
  cloudflaredStartFailedError,
  cloudflareNoAccountsError,
  cloudflareNoZoneError,
} from "../../src/errors/index.ts";

// ---------- error code uniqueness ----------

describe("error code uniqueness", () => {
  it("all error factory functions produce distinct codes per category", () => {
    const codes = new Set<string>();
    const errors = [
      invalidIntegerFlagError("test", "0", 1, 10),
      invalidRuntimeError("go", ["base"]),
      invalidNetworkPolicyError("invalid", ["allow-all"]),
      invalidPortError("bad"),
      portConflictError("80 (vm-1)"),
      invalidDomainError("bad"),
      invalidDomainPatternError("bad"),
      invalidCidrFormatError("bad"),
      invalidCidrPrefixError("10.0.0.0/33"),
      invalidCidrOctetError("256.0.0.0/24"),
      invalidImageRefEmptyError(),
      invalidImageRefTagError("img:"),
      invalidDiskSizeFormatError("bad"),
      invalidDiskSizeRangeError("0gb"),
      invalidDurationError("bad"),
      mutuallyExclusiveFlagsError("--a", "--b"),
      policyConflictError(),
      vmNotFoundError("vm-1"),
      vmStateNotFoundError("vm-1"),
      vmNotStoppedError("vm-1", "running"),
      vmNotRunningError("vm-1"),
      vmNoAgentTokenError("vm-1"),
      chrootNotFoundError("vm-1"),
      networkSlotsExhaustedError(),
      snapshotNotFoundError("snap-1"),
      firecrackerApiError("PUT", "/boot-source", 400, "invalid"),
      defaultInterfaceNotFoundError(),
      socketTimeoutError("/tmp/sock"),
      lockTimeoutError("state"),
      agentTimeoutError("198.19.0.2", 30000),
      missingBinaryError("firecracker", "/usr/local/bin/firecracker"),
      noKernelDirError(),
      noKernelError(),
      noRootfsDirError(),
      noExt4RootfsError(),
      cloudflareNotConfiguredError(),
      cloudflareTunnelNoIdError(),
      cloudflaredNotFoundError(),
      cloudflareConfigNotFoundError(),
      cloudflaredStartFailedError(),
      cloudflareNoAccountsError(),
      cloudflareNoZoneError("example.com"),
    ];

    for (const err of errors) {
      codes.add(err.code);
    }

    // There should be at least 20 unique codes
    expect(codes.size).toBeGreaterThanOrEqual(20);
  });
});

// ---------- error hierarchy ----------

describe("error hierarchy", () => {
  it("all errors extend VmsanError", () => {
    expect(invalidPortError("80")).toBeInstanceOf(VmsanError);
    expect(vmNotFoundError("vm-1")).toBeInstanceOf(VmsanError);
    expect(firecrackerApiError("GET", "/", 500, "")).toBeInstanceOf(VmsanError);
    expect(defaultInterfaceNotFoundError()).toBeInstanceOf(VmsanError);
    expect(socketTimeoutError("/sock")).toBeInstanceOf(VmsanError);
    expect(missingBinaryError("fc", "/bin/fc")).toBeInstanceOf(VmsanError);
    expect(cloudflareNotConfiguredError()).toBeInstanceOf(VmsanError);
  });

  it("all errors extend Error", () => {
    expect(invalidPortError("80")).toBeInstanceOf(Error);
    expect(vmNotFoundError("vm-1")).toBeInstanceOf(Error);
  });

  it("ValidationError has flag property", () => {
    const err = invalidPortError("80");
    expect(err).toBeInstanceOf(ValidationError);
    expect(err.flag).toBe("publish-port");
  });

  it("VmError has vmId property", () => {
    const err = vmNotFoundError("vm-abc");
    expect(err).toBeInstanceOf(VmError);
    expect(err.vmId).toBe("vm-abc");
  });

  it("TimeoutError has target and timeoutMs properties", () => {
    const err = agentTimeoutError("198.19.0.2", 5000);
    expect(err).toBeInstanceOf(TimeoutError);
    expect(err.target).toBe("198.19.0.2");
    expect(err.timeoutMs).toBe(5000);
  });

  it("FirecrackerApiError has method, path, httpStatus", () => {
    const err = firecrackerApiError("PUT", "/boot-source", 400, "bad");
    expect(err).toBeInstanceOf(FirecrackerApiError);
    expect(err.method).toBe("PUT");
    expect(err.path).toBe("/boot-source");
    expect(err.httpStatus).toBe(400);
  });

  it("CloudflareError has domain property", () => {
    const err = cloudflareNoZoneError("example.com");
    expect(err).toBeInstanceOf(CloudflareError);
    expect(err.domain).toBe("example.com");
  });
});

// ---------- fix suggestions ----------

describe("fix suggestions", () => {
  it("vmNotFoundError includes fix", () => {
    const err = vmNotFoundError("vm-1");
    expect(err.fix).toBeDefined();
    expect(err.fix).toContain("vmsan list");
  });

  it("vmNotStoppedError includes fix", () => {
    const err = vmNotStoppedError("vm-1", "running");
    expect(err.fix).toBeDefined();
    expect(err.fix).toContain("vmsan stop");
  });

  it("vmNotRunningError includes fix", () => {
    const err = vmNotRunningError("vm-1");
    expect(err.fix).toBeDefined();
    expect(err.fix).toContain("vmsan start");
  });

  it("chrootNotFoundError includes fix", () => {
    const err = chrootNotFoundError("vm-1");
    expect(err.fix).toBeDefined();
    expect(err.fix).toContain("vmsan create");
  });

  it("missingBinaryError includes install fix", () => {
    const err = missingBinaryError("firecracker", "/path");
    expect(err.fix).toBeDefined();
    expect(err.fix).toContain("install");
  });

  it("mutuallyExclusiveFlagsError includes fix", () => {
    const err = mutuallyExclusiveFlagsError("--snapshot", "--from-image");
    expect(err.fix).toBeDefined();
    expect(err.fix).toContain("--snapshot");
    expect(err.fix).toContain("--from-image");
  });

  it("cloudflaredNotFoundError includes fix", () => {
    const err = cloudflaredNotFoundError();
    expect(err.fix).toBeDefined();
  });
});

// ---------- error messages ----------

describe("error messages", () => {
  it("validation errors include the invalid value", () => {
    expect(invalidIntegerFlagError("vcpus", "999", 1, 32).message).toContain("999");
    expect(invalidRuntimeError("go", ["base"]).message).toContain("go");
    expect(invalidPortError("abc").message).toContain("abc");
    expect(invalidDomainError("bad domain").message).toContain("bad domain");
    expect(invalidCidrFormatError("bad").message).toContain("bad");
    expect(invalidImageRefTagError("img:").message).toContain("img:");
    expect(invalidDiskSizeFormatError("xyz").message).toContain("xyz");
    expect(invalidDurationError("xyz").message).toContain("xyz");
  });

  it("vm errors include the VM id", () => {
    expect(vmNotFoundError("vm-123").message).toContain("vm-123");
    expect(vmStateNotFoundError("vm-123").message).toContain("vm-123");
    expect(vmNotStoppedError("vm-123", "running").message).toContain("vm-123");
    expect(vmNotRunningError("vm-123").message).toContain("vm-123");
  });

  it("timeout errors include the target", () => {
    expect(socketTimeoutError("/tmp/api.sock").message).toContain("/tmp/api.sock");
    expect(lockTimeoutError("state").message).toContain("state");
    expect(agentTimeoutError("198.19.0.2", 5000).message).toContain("198.19.0.2");
  });
});

// ---------- toJSON ----------

describe("toJSON", () => {
  it("VmsanError.toJSON includes code", () => {
    const err = vmNotFoundError("vm-1");
    const json = err.toJSON();
    expect(json.code).toBe("ERR_VM_NOT_FOUND");
  });

  it("ValidationError.toJSON includes flag", () => {
    const err = invalidPortError("80");
    const json = err.toJSON();
    expect(json.flag).toBe("publish-port");
  });

  it("VmError.toJSON includes vmId", () => {
    const err = vmNotFoundError("vm-1");
    const json = err.toJSON();
    expect(json.vmId).toBe("vm-1");
  });

  it("FirecrackerApiError.toJSON includes method, path, httpStatus", () => {
    const err = firecrackerApiError("PUT", "/boot", 400, "bad");
    const json = err.toJSON();
    expect(json.method).toBe("PUT");
    expect(json.path).toBe("/boot");
    expect(json.httpStatus).toBe(400);
  });

  it("TimeoutError.toJSON includes target", () => {
    const err = socketTimeoutError("/sock");
    const json = err.toJSON();
    expect(json.target).toBe("/sock");
  });

  it("CloudflareError.toJSON includes domain", () => {
    const err = cloudflareNoZoneError("example.com");
    const json = err.toJSON();
    expect(json.domain).toBe("example.com");
  });
});

// ---------- handleCommandError JSON enrichment ----------

describe("handleCommandError JSON enrichment", () => {
  beforeEach(() => {
    // Enable evlog so createLogger returns a real logger with getContext()
    initLogger({ enabled: true, pretty: false, stringify: false });
  });

  function createTestCmdLog(): CommandLogger & { getContext: () => Record<string, unknown> } {
    const logger = createLogger({ path: "test" });
    return {
      set: logger.set.bind(logger),
      error: logger.error.bind(logger),
      emit: () => {},
      getContext: logger.getContext.bind(logger),
    };
  }

  it("includes error code in logger context for VmsanError", () => {
    const cmdLog = createTestCmdLog();
    const err = vmNotFoundError("vm-1");
    handleCommandError(err, cmdLog);

    const ctx = cmdLog.getContext();
    const error = ctx.error as Record<string, unknown>;
    expect(error.code).toBe("ERR_VM_NOT_FOUND");
  });

  it("includes vmId in logger context for VmError", () => {
    const cmdLog = createTestCmdLog();
    const err = vmNotFoundError("vm-1");
    handleCommandError(err, cmdLog);

    const ctx = cmdLog.getContext();
    const error = ctx.error as Record<string, unknown>;
    expect(error.vmId).toBe("vm-1");
  });

  it("includes flag in logger context for ValidationError", () => {
    const cmdLog = createTestCmdLog();
    const err = invalidPortError("bad");
    handleCommandError(err, cmdLog);

    const ctx = cmdLog.getContext();
    const error = ctx.error as Record<string, unknown>;
    expect(error.flag).toBe("publish-port");
  });

  it("promotes fix to error top level", () => {
    const cmdLog = createTestCmdLog();
    const err = vmNotFoundError("vm-1");
    handleCommandError(err, cmdLog);

    const ctx = cmdLog.getContext();
    const error = ctx.error as Record<string, unknown>;
    expect(error.fix).toBe("Run 'vmsan list' to see available VMs.");
  });

  it("promotes why to error top level", () => {
    const cmdLog = createTestCmdLog();
    const err = chrootNotFoundError("vm-1");
    handleCommandError(err, cmdLog);

    const ctx = cmdLog.getContext();
    const error = ctx.error as Record<string, unknown>;
    expect(error.why).toBe("The VM data may have been removed.");
  });

  it("preserves standard error fields (name, message, status)", () => {
    const cmdLog = createTestCmdLog();
    const err = vmNotFoundError("vm-1");
    handleCommandError(err, cmdLog);

    const ctx = cmdLog.getContext();
    const error = ctx.error as Record<string, unknown>;
    expect(error.name).toBe("VmError");
    expect(error.message).toContain("vm-1");
    expect(error.status).toBe(500);
  });

  it("does not add extra fields for non-VmsanError errors", () => {
    const cmdLog = createTestCmdLog();
    const err = new Error("generic error");
    handleCommandError(err, cmdLog);

    const ctx = cmdLog.getContext();
    const error = ctx.error as Record<string, unknown>;
    expect(error.name).toBe("Error");
    expect(error.message).toBe("generic error");
    expect(error).not.toHaveProperty("code");
  });
});
