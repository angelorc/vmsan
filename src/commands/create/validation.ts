import { FileVmStateStore } from "../../lib/vm-state.ts";
import type { VmsanPaths } from "../../paths.ts";
import {
  VALID_NETWORK_POLICIES,
  VALID_RUNTIMES,
  type NetworkPolicy,
  type Runtime,
} from "./types.ts";
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
} from "../../errors/index.ts";

function parseIntegerFlag(
  flagName: string,
  value: string | undefined,
  fallback: string,
  min: number,
  max: number,
  unitSuffix: string = "",
): number {
  const raw = value ?? fallback;
  const parsed = Number(raw);
  if (!Number.isInteger(parsed) || parsed < min || parsed > max) {
    throw invalidIntegerFlagError(flagName, raw, min, max, unitSuffix);
  }
  return parsed;
}

export function parseVcpuCount(value: string | undefined): number {
  return parseIntegerFlag("vcpus", value, "1", 1, 32);
}

export function parseMemoryMib(value: string | undefined): number {
  return parseIntegerFlag("memory", value, "128", 128, 32768, " MiB");
}

export function parseRuntime(value: string | undefined): Runtime {
  const runtime = value || "base";
  if (!VALID_RUNTIMES.includes(runtime as Runtime)) {
    throw invalidRuntimeError(runtime, VALID_RUNTIMES);
  }
  return runtime as Runtime;
}

export function parseNetworkPolicy(value: string | undefined): NetworkPolicy {
  const policy = value || "allow-all";
  if (!VALID_NETWORK_POLICIES.includes(policy as NetworkPolicy)) {
    throw invalidNetworkPolicyError(policy, VALID_NETWORK_POLICIES);
  }
  return policy as NetworkPolicy;
}

export function parsePublishedPorts(value: string | undefined): number[] {
  if (!value) return [];
  return value.split(",").map((rawPort) => {
    const port = Number(rawPort.trim());
    if (Number.isNaN(port) || port < 1 || port > 65535) {
      throw invalidPortError(rawPort.trim());
    }
    return port;
  });
}

const DOMAIN_LABEL_REGEX = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/i;

function validateDomainPattern(domain: string): string {
  if (!domain) return domain;

  if (domain.includes("\n") || domain.includes("\r") || /\s/.test(domain)) {
    throw invalidDomainError(domain);
  }

  const normalized = domain.toLowerCase();
  const wildcardCount = (normalized.match(/\*/g) || []).length;
  if (wildcardCount > 0 && !normalized.startsWith("*.")) {
    throw invalidDomainPatternError(
      domain,
      'Wildcards are only supported as a leading "*." prefix.',
    );
  }
  if (wildcardCount > 1) {
    throw invalidDomainPatternError(domain);
  }

  const zone = normalized.startsWith("*.") ? normalized.slice(2) : normalized;
  if (!zone || zone.length > 253) {
    throw invalidDomainError(domain);
  }

  const labels = zone.split(".");
  if (labels.some((label) => !DOMAIN_LABEL_REGEX.test(label))) {
    throw invalidDomainError(domain);
  }

  return normalized;
}

export function parseDomains(value: string | undefined): string[] {
  if (!value) return [];
  return value
    .split(",")
    .map((domain) => validateDomainPattern(domain.trim()))
    .filter(Boolean);
}

const CIDR_REGEX = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/;

export function parseCidrList(value: string | undefined): string[] {
  if (!value) return [];
  return value
    .split(",")
    .map((cidr) => cidr.trim())
    .filter(Boolean);
}

export function validateCidr(cidr: string): void {
  if (!CIDR_REGEX.test(cidr)) {
    throw invalidCidrFormatError(cidr);
  }
  const [ip, prefix] = cidr.split("/");
  const prefixLen = Number(prefix);
  if (prefixLen < 0 || prefixLen > 32) {
    throw invalidCidrPrefixError(cidr);
  }
  const octets = ip.split(".").map(Number);
  for (const octet of octets) {
    if (octet < 0 || octet > 255) {
      throw invalidCidrOctetError(cidr);
    }
  }
}

export function validatePublishedPortsAvailable(ports: number[], paths: VmsanPaths): void {
  if (ports.length === 0) return;

  const collisions = new Map<number, string[]>();
  const store = new FileVmStateStore(paths.vmsDir);
  const activeStates = store
    .list()
    .filter((state) => state.status === "running" || state.status === "creating");

  for (const state of activeStates) {
    for (const usedPort of state.network.publishedPorts ?? []) {
      if (!ports.includes(usedPort)) continue;
      const existing = collisions.get(usedPort) ?? [];
      existing.push(state.id);
      collisions.set(usedPort, existing);
    }
  }

  if (collisions.size === 0) return;

  const conflictSummary = [...collisions.entries()]
    .map(([port, vmIds]) => `${port} (in use by ${vmIds.join(", ")})`)
    .join(", ");
  throw portConflictError(conflictSummary);
}

export interface ImageReference {
  full: string;
  name: string;
  tag: string;
  cacheKey: string;
}

export function parseImageReference(ref: string): ImageReference {
  const trimmed = ref.trim();
  if (!trimmed) {
    throw invalidImageRefEmptyError();
  }

  let name: string;
  let tag: string;

  const lastColon = trimmed.lastIndexOf(":");
  if (lastColon > 0 && !trimmed.substring(lastColon).includes("/")) {
    name = trimmed.substring(0, lastColon);
    tag = trimmed.substring(lastColon + 1);
  } else {
    name = trimmed;
    tag = "latest";
  }

  if (!tag) {
    throw invalidImageRefTagError(ref);
  }

  const full = `${name}:${tag}`;
  const cacheKey = `${name}/${tag}`.replace(/:/g, "/");

  return { full, name, tag, cacheKey };
}

export function parseBandwidth(value: string | undefined): number | undefined {
  if (!value) return undefined;
  const raw = value.trim().toLowerCase();
  const match = raw.match(/^(\d+)(mbit|m)?$/i);
  if (!match) {
    throw invalidIntegerFlagError("bandwidth", value, 1, 1000, " mbit");
  }
  const mbit = Number(match[1]);
  if (!Number.isInteger(mbit) || mbit < 1 || mbit > 1000) {
    throw invalidIntegerFlagError("bandwidth", value, 1, 1000, " mbit");
  }
  return mbit;
}

export function parseDiskSizeGb(value: string | undefined): number {
  const raw = (value || "10gb").trim().toLowerCase();
  const match = raw.match(/^(\d+)(gb|g|gib)?$/i);
  if (!match) {
    throw invalidDiskSizeFormatError(value!);
  }
  const sizeGb = Number(match[1]);
  if (!Number.isInteger(sizeGb) || sizeGb < 1 || sizeGb > 1024) {
    throw invalidDiskSizeRangeError(value!);
  }
  return sizeGb;
}
