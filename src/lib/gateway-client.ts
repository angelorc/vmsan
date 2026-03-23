import { connect } from "node:net";
import { existsSync } from "node:fs";
import { spawn } from "node:child_process";
import process from "node:process";
import { consola } from "consola";

// ---------------------------------------------------------------------------
// Timeouts (ms) — per-method instead of global 10s
// ---------------------------------------------------------------------------

const TIMEOUT_PING = 5_000;
const TIMEOUT_STATUS = 5_000;
const TIMEOUT_HEALTH = 5_000;
const TIMEOUT_CREATE = 120_000;
const TIMEOUT_RESTART = 120_000;
const TIMEOUT_STOP = 30_000;
const TIMEOUT_DELETE = 30_000;
const TIMEOUT_SNAPSHOT = 60_000;
const TIMEOUT_ROOTFS_BUILD = 120_000;
const TIMEOUT_UPDATE_POLICY = 10_000;
const TIMEOUT_SHUTDOWN = 5_000;

// ---------------------------------------------------------------------------
// Existing interfaces (backward-compatible)
// ---------------------------------------------------------------------------

export interface GatewayVmConfig {
  vmId: string;
  slot: number;
  policy: string;
  allowedDomains?: string[];
  project?: string;
  service?: string;
  connectTo?: string[];
  vethHost?: string;
  netns?: string;
  guestDev?: string;
}

export interface GatewayVmResult {
  ok: boolean;
  error?: string;
  code?: string;
  vm?: {
    vmId: string;
    slot: number;
    policy: string;
    dnsPort: number;
    sniPort: number;
    httpPort: number;
    meshIp?: string;
  };
}

export interface GatewayPingResult {
  ok: boolean;
  version: string;
  vms: number;
}

// ---------------------------------------------------------------------------
// New interfaces — matching Go param/response structs
// ---------------------------------------------------------------------------

export interface GatewayVmCreateParams {
  vcpus?: number;
  memMib?: number;
  runtime?: string;
  diskSizeGb?: number;
  networkPolicy?: string;
  domains?: string[];
  allowedCidrs?: string[];
  deniedCidrs?: string[];
  ports?: number[];
  bandwidthMbit?: number;
  allowIcmp?: boolean;
  project?: string;
  service?: string;
  connectTo?: string[];
  skipDnat?: boolean;
  kernelPath?: string;
  rootfsPath?: string;
  snapshotId?: string;
  agentBinary?: string;
  agentToken?: string;
  vmId?: string;
  disableSeccomp?: boolean;
  disablePidNs?: boolean;
  disableCgroup?: boolean;
  seccompFilter?: string;
  ownerUid?: number;
  ownerGid?: number;
}

export interface GatewayVmCreateResult {
  ok: boolean;
  error?: string;
  code?: string;
  vm?: {
    vmId: string;
    slot: number;
    hostIp: string;
    guestIp: string;
    meshIp?: string;
    tapDevice: string;
    macAddress: string;
    netnsName: string;
    vethHost: string;
    vethGuest: string;
    subnetMask: string;
    chrootDir: string;
    socketPath: string;
    pid: number;
    agentToken?: string;
    dnsPort: number;
    sniPort: number;
    httpPort: number;
  };
}

export interface GatewayVmRestartParams {
  vmId: string;
  slot: number;
  chrootDir?: string;
  socketPath?: string;
  networkPolicy?: string;
  domains?: string[];
  allowedCidrs?: string[];
  deniedCidrs?: string[];
  ports?: number[];
  bandwidthMbit?: number;
  allowIcmp?: boolean;
  skipDnat?: boolean;
  project?: string;
  service?: string;
  connectTo?: string[];
  disableSeccomp?: boolean;
  disablePidNs?: boolean;
  disableCgroup?: boolean;
  seccompFilter?: string;
  vcpus?: number;
  memMib?: number;
  kernelPath?: string;
  rootfsPath?: string;
  agentBinary?: string;
  agentToken?: string;
  netnsName?: string;
}

export interface GatewayVmFullStopParams {
  vmId: string;
  slot?: number;
  pid?: number;
  netnsName?: string;
  socketPath?: string;
}

export interface GatewayVmDeleteParams {
  vmId: string;
  force?: boolean;
}

export interface GatewayUpdatePolicyParams {
  vmId: string;
  policy: string;
  slot?: number;
  domains?: string[];
  allowedCidrs?: string[];
  deniedCidrs?: string[];
  ports?: number[];
  allowIcmp?: boolean;
  skipDnat?: boolean;
  netnsName?: string;
}

export interface GatewaySnapshotCreateParams {
  vmId: string;
  snapshotId: string;
  socketPath: string;
  destDir: string;
  chrootDir?: string;
  ownerUid?: number;
  ownerGid?: number;
}

export interface GatewayRootfsBuildParams {
  imageRef: string;
  outputDir: string;
  ownerUid?: number;
  ownerGid?: number;
}

export interface GatewayGenericResult {
  ok: boolean;
  error?: string;
  code?: string;
  vm?: Record<string, unknown>;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getOwnerUid(): number {
  return process.getuid?.() ?? 0;
}

function getOwnerGid(): number {
  return process.getgid?.() ?? 0;
}

// ---------------------------------------------------------------------------
// GatewayClient
// ---------------------------------------------------------------------------

export class GatewayClient {
  constructor(private socketPath: string = "/run/vmsan/gateway.sock") {}

  // -- Existing methods (backward-compatible) --

  async vmStart(config: GatewayVmConfig): Promise<GatewayVmResult> {
    return this.send("vm.start", config, TIMEOUT_CREATE);
  }

  async vmStop(vmId: string): Promise<void> {
    await this.send("vm.stop", { vmId }, TIMEOUT_STOP);
  }

  async vmUpdatePolicy(vmId: string, policy: string, allowedDomains?: string[]): Promise<void> {
    await this.send("vm.updatePolicy", { vmId, policy, allowedDomains }, TIMEOUT_UPDATE_POLICY);
  }

  async ping(): Promise<GatewayPingResult> {
    return this.send("ping", undefined, TIMEOUT_PING);
  }

  async shutdown(): Promise<void> {
    await this.send("shutdown", undefined, TIMEOUT_SHUTDOWN);
  }

  // -- New methods (full lifecycle) --

  async vmCreate(params: GatewayVmCreateParams): Promise<GatewayVmCreateResult> {
    return this.send("vm.create", {
      ...params,
      ownerUid: params.ownerUid ?? getOwnerUid(),
      ownerGid: params.ownerGid ?? getOwnerGid(),
    }, TIMEOUT_CREATE);
  }

  async vmRestart(params: GatewayVmRestartParams): Promise<GatewayVmCreateResult> {
    return this.send("vm.restart", params, TIMEOUT_RESTART);
  }

  async vmFullStop(params: GatewayVmFullStopParams): Promise<GatewayGenericResult> {
    return this.send("vm.fullStop", params, TIMEOUT_STOP);
  }

  async vmDelete(params: GatewayVmDeleteParams): Promise<GatewayGenericResult> {
    return this.send("vm.delete", params, TIMEOUT_DELETE);
  }

  async vmFullUpdatePolicy(params: GatewayUpdatePolicyParams): Promise<GatewayGenericResult> {
    return this.send("vm.fullUpdatePolicy", params, TIMEOUT_UPDATE_POLICY);
  }

  async snapshotCreate(params: GatewaySnapshotCreateParams): Promise<GatewayGenericResult> {
    return this.send("vm.snapshot.create", {
      ...params,
      ownerUid: params.ownerUid ?? getOwnerUid(),
      ownerGid: params.ownerGid ?? getOwnerGid(),
    }, TIMEOUT_SNAPSHOT);
  }

  async rootfsBuild(params: GatewayRootfsBuildParams): Promise<GatewayGenericResult> {
    return this.send("rootfs.build", {
      ...params,
      ownerUid: params.ownerUid ?? getOwnerUid(),
      ownerGid: params.ownerGid ?? getOwnerGid(),
    }, TIMEOUT_ROOTFS_BUILD);
  }

  // -- Internal --

  private async send(method: string, params?: unknown, timeoutMs: number = 10_000): Promise<any> {
    return new Promise((resolve, reject) => {
      const socket = connect(this.socketPath);
      let data = "";

      socket.on("connect", () => {
        const request = JSON.stringify({ method, params });
        socket.write(request + "\n");
      });

      socket.on("data", (chunk) => {
        data += chunk.toString();
      });

      socket.on("end", () => {
        try {
          const response = JSON.parse(data);
          resolve(response);
        } catch {
          reject(new Error(`Failed to parse gateway response: ${data}`));
        }
      });

      socket.on("error", (err) => {
        reject(new Error(`Gateway connection error: ${err.message}`));
      });

      socket.setTimeout(timeoutMs);
      socket.on("timeout", () => {
        socket.destroy();
        reject(new Error(`Gateway connection timeout after ${timeoutMs}ms for method: ${method}`));
      });
    });
  }
}

/**
 * Ensure vmsan-gateway is running. Throws if the gateway can't be reached.
 * The gateway daemon is required for all VM operations — without it, nothing works.
 */
export async function ensureGatewayRunning(gatewayBin: string): Promise<void> {
  const client = new GatewayClient();
  try {
    await client.ping();
    return; // Already running
  } catch {
    // Not running, try to start it
  }

  // Try systemctl first (preferred for systemd-managed daemon)
  try {
    const { execSync } = await import("node:child_process");
    execSync("systemctl start vmsan-gateway", { stdio: "pipe", timeout: 5000 });
    // Wait for socket to appear
    for (let i = 0; i < 25; i++) {
      await new Promise((r) => setTimeout(r, 200));
      try {
        await client.ping();
        return;
      } catch {
        // Still starting
      }
    }
  } catch {
    // systemctl failed (not installed, no service file, etc.)
  }

  // Fallback: direct spawn
  if (!existsSync(gatewayBin)) {
    throw new Error(
      "vmsan-gateway binary not found. Install it with: curl -fsSL https://vmsan.dev/install | bash",
    );
  }

  const child = spawn(gatewayBin, ["start"], {
    detached: true,
    stdio: "ignore",
  });
  child.unref();

  // Wait for it to be ready (up to 5 seconds)
  for (let i = 0; i < 25; i++) {
    await new Promise((r) => setTimeout(r, 200));
    try {
      await client.ping();
      return;
    } catch {
      // Still starting
    }
  }

  throw new Error(
    "vmsan-gateway failed to start within 5 seconds. Check: sudo systemctl status vmsan-gateway",
  );
}
