import { createHash } from "node:crypto";
import { readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import consola from "consola";
import type { VMService, CreateVmOptions } from "../../services/vm.ts";
import type { VmState } from "../vm-state.ts";
import type { ServiceConfig, DeployConfig, AccessoryConfig } from "../toml/parser.ts";
import { uploadSource } from "./upload.ts";
import { executeBuild, startApp } from "./build.ts";
import { executeRelease } from "./release.ts";
import { getDeployHash, setDeployHash } from "./hash.ts";
import { toError } from "../utils.ts";
import { waitForAgent } from "../vm-context.ts";
import { AgentClient } from "../../services/agent.ts";
import { getRootfsPath, type RootfsType } from "../rootfs-manager.ts";

// ── Types ───────────────────────────────────────────────────────────────────

export type DeployStatus =
  | "pending"
  | "creating"
  | "uploading"
  | "building"
  | "releasing"
  | "starting"
  | "health_check"
  | "running"
  | "failed"
  | "skipped";

export interface ServiceDeployResult {
  service: string;
  status: DeployStatus;
  vmId: string | null;
  error?: string;
  skipped?: boolean;
  durationMs: number;
}

export interface DeployServiceOptions {
  /** Service name */
  name: string;
  /** Service config from vmsan.toml */
  config: ServiceConfig;
  /** Deploy config from vmsan.toml */
  deployConfig?: DeployConfig;
  /** Project name (from vmsan.toml or directory name) */
  project: string;
  /** Source directory */
  sourceDir: string;
  /** VMService instance */
  vmService: VMService;
  /** Environment variables to inject */
  env?: Record<string, string>;
  /** Status callback */
  onStatus?: (status: DeployStatus) => void;
  /** Whether this is an accessory (skip upload/build) */
  accessory?: AccessoryConfig;
}

// ── Helpers ─────────────────────────────────────────────────────────────────

const ACCESSORY_ROOTFS_MAP: Record<string, RootfsType> = {
  postgres: "postgres16",
  redis: "redis7",
};

function parsePublishPorts(portSpecs?: string[]): number[] {
  if (!portSpecs || portSpecs.length === 0) return [];
  const ports: number[] = [];
  for (const spec of portSpecs) {
    // Format: "8080:8080" or "8080"
    const parts = spec.split(":");
    const hostPort = Number(parts[0]);
    if (!Number.isNaN(hostPort) && hostPort > 0) {
      ports.push(hostPort);
    }
  }
  return ports;
}

/**
 * Compute a deploy hash from source directory contents and service config.
 * Used to detect whether a re-deploy is needed.
 */
function computeDeployHash(sourceDir: string, serviceConfig: ServiceConfig): string {
  const hash = createHash("sha256");
  // Hash the service config deterministically
  hash.update(JSON.stringify(serviceConfig, Object.keys(serviceConfig).sort()));
  // Hash source file listing with sizes (fast approximation)
  walkForHash(sourceDir, hash);
  return hash.digest("hex");
}

function walkForHash(dir: string, hash: ReturnType<typeof createHash>): void {
  let names: string[];
  try {
    names = readdirSync(dir);
  } catch {
    return;
  }

  const SKIP = new Set([".git", "node_modules", ".vmsan"]);
  names.sort();

  for (const name of names) {
    if (SKIP.has(name)) continue;
    const fullPath = join(dir, name);
    try {
      const stat = statSync(fullPath);
      if (stat.isDirectory()) {
        hash.update(`d:${name}\n`);
        walkForHash(fullPath, hash);
      } else if (stat.isFile()) {
        hash.update(`f:${name}:${stat.size}:${stat.mtimeMs}\n`);
      }
    } catch {
      hash.update(`f:${name}\n`);
    }
  }
}

/**
 * Find an existing VM for a given project + service name.
 */
function findExistingVm(
  vmService: VMService,
  project: string,
  serviceName: string,
): VmState | null {
  const vms = vmService.list();
  for (const vm of vms) {
    if (vm.project === project && vm.network.service === serviceName && vm.status !== "error") {
      return vm;
    }
  }
  return null;
}

/**
 * Create an AgentClient for a VM.
 */
function createAgentClient(state: VmState): AgentClient {
  const agentUrl = `http://${state.network.guestIp}:${state.agentPort}`;
  return new AgentClient(agentUrl, state.agentToken!);
}

// ── Health check polling ────────────────────────────────────────────────────

const HEALTH_CHECK_TIMEOUT_MS = 90_000;
const HEALTH_CHECK_POLL_INTERVAL_MS = 2_000;

async function configureHealthCheck(
  agentUrl: string,
  agentToken: string,
  config: ServiceConfig["health_check"],
): Promise<void> {
  if (!config) return;
  const res = await fetch(`${agentUrl}/health/configure`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${agentToken}`,
    },
    body: JSON.stringify(config),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Failed to configure health check: ${res.status} ${text}`);
  }
}

async function waitForHealthy(
  agentUrl: string,
  agentToken: string,
  timeoutMs = HEALTH_CHECK_TIMEOUT_MS,
): Promise<boolean> {
  const start = Date.now();

  while (Date.now() - start < timeoutMs) {
    try {
      const res = await fetch(`${agentUrl}/health`, {
        headers: { Authorization: `Bearer ${agentToken}` },
        signal: AbortSignal.timeout(2000),
      });
      if (res.ok) {
        const data = (await res.json()) as { status?: string };
        if (data.status === "healthy" || data.status === "ok") {
          return true;
        }
      }
    } catch {
      // Not ready yet
    }
    await new Promise((r) => setTimeout(r, HEALTH_CHECK_POLL_INTERVAL_MS));
  }
  return false;
}

// ── Main deploy function ────────────────────────────────────────────────────

export async function deployService(opts: DeployServiceOptions): Promise<ServiceDeployResult> {
  const { name, config, deployConfig, project, sourceDir, vmService, env, onStatus, accessory } = opts;
  const startTime = Date.now();
  const isAccessory = !!accessory;

  const emitStatus = (status: DeployStatus): void => {
    onStatus?.(status);
  };

  try {
    // 1. Check existing VM and deploy hash
    emitStatus("creating");
    consola.start(`[${name}] Checking deployment state...`);

    const existingVm = findExistingVm(vmService, project, name);
    let deployHash: string | null = null;

    if (!isAccessory) {
      deployHash = computeDeployHash(sourceDir, config);
    }

    if (existingVm && deployHash) {
      const storedHash = getDeployHash(existingVm.id);
      if (storedHash === deployHash) {
        consola.success(`[${name}] No changes detected, skipping`);
        emitStatus("skipped");
        return {
          service: name,
          status: "skipped",
          vmId: existingVm.id,
          skipped: true,
          durationMs: Date.now() - startTime,
        };
      }
      // Hash differs: stop and remove existing VM for re-deploy
      consola.info(`[${name}] Changes detected, re-deploying`);
      await vmService.remove(existingVm.id, { force: true });
    } else if (existingVm && isAccessory) {
      // Accessories without hash tracking: skip if running
      if (existingVm.status === "running") {
        consola.success(`[${name}] Accessory already running, skipping`);
        emitStatus("skipped");
        return {
          service: name,
          status: "skipped",
          vmId: existingVm.id,
          skipped: true,
          durationMs: Date.now() - startTime,
        };
      }
    }

    // 2. Create VM
    consola.start(`[${name}] Creating VM...`);

    const createOpts: CreateVmOptions = {
      project,
      runtime: (config.runtime as CreateVmOptions["runtime"]) ?? "base",
      vcpus: config.vcpus ?? 1,
      memMib: config.memory ?? 512,
      networkPolicy: (config.network_policy as CreateVmOptions["networkPolicy"]) ?? "allow-all",
      domains: config.allowed_domains,
      service: name,
      connectTo: config.connect_to,
      ports: parsePublishPorts(config.publish_ports),
    };

    // For accessories, use the pre-built rootfs
    if (isAccessory && accessory.type) {
      const rootfsType = ACCESSORY_ROOTFS_MAP[accessory.type];
      if (rootfsType) {
        createOpts.rootfsPath = await getRootfsPath(rootfsType);
      }
    }

    const { vmId, state } = await vmService.create(createOpts);
    consola.success(`[${name}] VM ${vmId} created`);

    // Wait for agent to be available
    if (state.agentToken) {
      consola.start(`[${name}] Waiting for agent...`);
      await waitForAgent(state.network.guestIp, state.agentPort);
      consola.success(`[${name}] Agent ready`);
    }

    // For accessories, we're done after creating the VM
    if (isAccessory) {
      emitStatus("running");
      consola.success(`[${name}] Accessory deployed`);
      return {
        service: name,
        status: "running",
        vmId,
        durationMs: Date.now() - startTime,
      };
    }

    if (!state.agentToken) {
      throw new Error(`VM ${vmId} has no agent token`);
    }

    const agentUrl = `http://${state.network.guestIp}:${state.agentPort}`;
    const agentToken = state.agentToken;
    const agent = createAgentClient(state);

    // 3. Upload source
    emitStatus("uploading");
    consola.start(`[${name}] Uploading source...`);
    await uploadSource({ sourceDir, agent, targetDir: "/app" });

    // 4. Build
    if (config.build) {
      emitStatus("building");
      consola.start(`[${name}] Running build...`);
      const buildResult = await executeBuild({
        buildCommand: config.build,
        agent,
        cwd: "/app",
        env: { ...config.env, ...env },
      });
      if (!buildResult.success) {
        throw new Error(`Build failed with exit code ${buildResult.exitCode}`);
      }
    }

    // 5. Release
    if (deployConfig?.release) {
      emitStatus("releasing");
      consola.start(`[${name}] Running release command...`);
      const releaseResult = await executeRelease({
        command: deployConfig.release,
        agent,
      });
      if (!releaseResult.success) {
        throw new Error(`Release command failed with exit code ${releaseResult.exitCode}`);
      }
    }

    // 6. Start app
    if (config.start) {
      emitStatus("starting");
      consola.start(`[${name}] Starting app...`);
      await startApp({
        startCommand: config.start,
        agent,
        cwd: "/app",
        env: { ...config.env, ...env },
      });
    }

    // 7. Health check
    if (config.health_check) {
      emitStatus("health_check");
      consola.start(`[${name}] Waiting for health check...`);
      try {
        await configureHealthCheck(agentUrl, agentToken, config.health_check);
      } catch (err) {
        consola.debug(`[${name}] Failed to configure health check: ${toError(err).message}`);
      }
      const healthy = await waitForHealthy(agentUrl, agentToken);
      if (!healthy) {
        throw new Error("Health check timed out after 90s");
      }
      consola.success(`[${name}] Health check passed`);
    }

    // 8. Done
    emitStatus("running");

    // Store deploy hash for skip detection on next deploy
    if (deployHash) {
      setDeployHash(vmId, deployHash);
    }

    consola.success(`[${name}] Deployed successfully`);
    return {
      service: name,
      status: "running",
      vmId,
      durationMs: Date.now() - startTime,
    };
  } catch (err) {
    emitStatus("failed");
    const errorMessage = toError(err).message;
    consola.error(`[${name}] Deploy failed: ${errorMessage}`);
    return {
      service: name,
      status: "failed",
      vmId: null,
      error: errorMessage,
      durationMs: Date.now() - startTime,
    };
  }
}
