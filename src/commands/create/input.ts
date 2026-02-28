import type { VmsanPaths } from "../../paths.ts";
import { parseDuration } from "../../lib/utils.ts";
import { assertSnapshotExists } from "./environment.ts";
import type { ParsedCreateInput } from "./types.ts";
import {
  parseCidrList,
  parseDiskSizeGb,
  parseDomains,
  parseMemoryMib,
  parseNetworkPolicy,
  parsePublishedPorts,
  parseRuntime,
  parseVcpuCount,
  validateCidr,
  validatePublishedPortsAvailable,
} from "./validation.ts";
import { policyConflictError } from "../../errors/index.ts";

export interface CreateCommandRuntimeArgs {
  vcpus?: string;
  memory?: string;
  runtime?: string;
  disk?: string;
  timeout?: string;
  snapshot?: string;
  "network-policy"?: string;
  "publish-port"?: string;
  "allowed-domain"?: string;
  "allowed-cidr"?: string;
  "denied-cidr"?: string;
}

export function parseCreateInput(args: CreateCommandRuntimeArgs, paths: VmsanPaths): ParsedCreateInput {
  const vcpus = parseVcpuCount(args.vcpus);
  const memMib = parseMemoryMib(args.memory);
  const runtime = parseRuntime(args.runtime);
  const networkPolicy = parseNetworkPolicy(args["network-policy"]);
  const ports = parsePublishedPorts(args["publish-port"]);
  const domains = parseDomains(args["allowed-domain"]);
  const allowedCidrs = parseCidrList(args["allowed-cidr"]);
  const deniedCidrs = parseCidrList(args["denied-cidr"]);
  const timeoutMs = typeof args.timeout === "string" ? parseDuration(args.timeout) : null;
  const snapshotId = typeof args.snapshot === "string" ? args.snapshot : null;
  const diskSizeGb = parseDiskSizeGb(args.disk);

  validatePublishedPortsAvailable(ports, paths);

  for (const cidr of allowedCidrs) validateCidr(cidr);
  for (const cidr of deniedCidrs) validateCidr(cidr);

  if (
    networkPolicy === "deny-all" &&
    (domains.length || allowedCidrs.length || deniedCidrs.length)
  ) {
    throw policyConflictError();
  }

  if (snapshotId) {
    assertSnapshotExists(snapshotId, paths);
  }

  return {
    vcpus,
    memMib,
    runtime,
    networkPolicy,
    ports,
    domains,
    allowedCidrs,
    deniedCidrs,
    timeoutMs,
    snapshotId,
    diskSizeGb,
  };
}
