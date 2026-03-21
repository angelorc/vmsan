import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { consola } from "consola";
import { defaultInterfaceNotFoundError } from "../errors/index.ts";
import { vmsanPaths } from "../paths.ts";
import { GatewayClient, ensureGatewayRunning } from "./gateway-client.ts";
import { toError } from "./utils.ts";
import type { VmNetwork } from "./vm-state.ts";
import {
  slotFromVmHostIp,
  slotFromVmHostIpOrNull,
  vmGuestIp,
  vmHostIp,
  vmLinkCidrFromIp,
  VM_SUBNET_MASK,
} from "./network-address.ts";

export interface NetworkConfig {
  slot: number;
  tapDevice: string;
  hostIp: string;
  guestIp: string;
  subnetMask: string;
  macAddress: string;
  networkPolicy: string;
  allowedDomains: string[];
  allowedCidrs: string[];
  deniedCidrs: string[];
  publishedPorts: number[];
  bandwidthMbit?: number;
  netnsName?: string;
  skipDnat?: boolean;
  allowIcmp?: boolean;
  /** VM identifier, used by nftables backend for per-VM table naming. */
  vmId?: string;
}

// DNS resolvers the VM uses (Google Public DNS)
const DNS_RESOLVERS = ["8.8.8.8", "8.8.4.4"];

// ---------------------------------------------------------------------------
// nftables binary helper
// ---------------------------------------------------------------------------

interface NftablesResult {
  ok: boolean;
  error?: string;
  code?: string;
  tableExists?: boolean;
  chainCount?: number;
}

function execNftables(command: string, config: object): NftablesResult {
  const nftablesPath = join(vmsanPaths().binDir, "vmsan-nftables");
  const input = JSON.stringify(config);
  try {
    const result = execFileSync(nftablesPath, [command], {
      input,
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    });
    return JSON.parse(result);
  } catch (err: unknown) {
    // If the binary exits non-zero, execFileSync throws.
    // Try to parse stdout for structured error.
    const error = toError(err);
    if (
      err !== null &&
      typeof err === "object" &&
      "stdout" in err &&
      typeof (err as { stdout: unknown }).stdout === "string" &&
      (err as { stdout: string }).stdout.trim()
    ) {
      try {
        return JSON.parse((err as { stdout: string }).stdout);
      } catch {
        // fall through
      }
    }
    return { ok: false, error: error.message, code: "ERR_NFT_EXEC" };
  }
}

/**
 * Derive vmId from the NetworkConfig. Prefers the explicit vmId field,
 * falls back to extracting from the netnsName pattern "vmsan-<vmId>".
 */
function resolveVmId(config: NetworkConfig): string {
  if (config.vmId) return config.vmId;
  if (config.netnsName?.startsWith("vmsan-")) {
    return config.netnsName.slice("vmsan-".length);
  }
  return `slot-${config.slot}`;
}

// ---------------------------------------------------------------------------
// Process helpers
// ---------------------------------------------------------------------------

function runArgs(bin: string, args: string[]): void {
  execFileSync(bin, args, { stdio: "pipe" });
}

function sudo(args: string[]): void {
  runArgs("sudo", args);
}

function sudoNetns(nsName: string, args: string[]): void {
  sudo(["ip", "netns", "exec", nsName, ...args]);
}

function getDefaultInterface(): string {
  const output = execFileSync("ip", ["route", "show", "default"], {
    encoding: "utf-8",
    stdio: "pipe",
  }).trim();
  const match = output.match(/dev\s+(\S+)/);
  if (!match) {
    throw defaultInterfaceNotFoundError();
  }
  return match[1];
}

/**
 * Detect effective policy mode:
 * - "deny-all": explicit deny-all, no outbound
 * - "custom": any of allowedDomains/allowedCidrs/deniedCidrs present
 * - "allow-all": default, unrestricted
 */
function effectivePolicy(config: NetworkConfig): string {
  if (config.networkPolicy === "deny-all") return "deny-all";
  if (
    config.allowedDomains.length > 0 ||
    config.allowedCidrs.length > 0 ||
    config.deniedCidrs.length > 0
  ) {
    return "custom";
  }
  return "allow-all";
}

export class NetworkManager {
  config: NetworkConfig;

  constructor(config: NetworkConfig) {
    this.config = config;
  }

  /** Create a NetworkManager from a slot and policy options. */
  static fromSlot(
    slot: number,
    opts: {
      networkPolicy: string;
      allowedDomains: string[];
      allowedCidrs: string[];
      deniedCidrs: string[];
      publishedPorts: number[];
      bandwidthMbit?: number;
      netnsName?: string;
      skipDnat?: boolean;
      allowIcmp?: boolean;
    },
  ): NetworkManager {
    return new NetworkManager({
      slot,
      tapDevice: `fhvm${slot}`,
      hostIp: vmHostIp(slot),
      guestIp: vmGuestIp(slot),
      subnetMask: VM_SUBNET_MASK,
      macAddress: `AA:FC:00:00:00:${(slot + 1).toString(16).padStart(2, "0").toUpperCase()}`,
      ...opts,
    });
  }

  static bootArgs(config: Pick<NetworkConfig, "guestIp" | "hostIp" | "subnetMask">): string {
    return `console=ttyS0 reboot=k panic=1 pci=off ip=${config.guestIp}::${config.hostIp}:${config.subnetMask}::eth0:off:${DNS_RESOLVERS[0]}`;
  }

  static fromConfig(config: NetworkConfig): NetworkManager {
    return new NetworkManager(config);
  }

  static fromVmNetwork(network: VmNetwork): NetworkManager {
    let slot: number;
    try {
      slot = slotFromVmHostIp(network.hostIp);
    } catch {
      throw new Error(`invalid network slot derived from hostIp: ${network.hostIp}`);
    }
    return NetworkManager.fromConfig({
      slot,
      tapDevice: network.tapDevice,
      hostIp: network.hostIp,
      guestIp: network.guestIp,
      subnetMask: network.subnetMask,
      macAddress: network.macAddress,
      networkPolicy: network.networkPolicy,
      allowedDomains: network.allowedDomains,
      allowedCidrs: network.allowedCidrs || [],
      deniedCidrs: network.deniedCidrs || [],
      publishedPorts: network.publishedPorts,
      bandwidthMbit: network.bandwidthMbit,
      netnsName: network.netnsName,
      skipDnat: network.skipDnat,
      allowIcmp: network.allowIcmp,
    });
  }

  private nsRun(args: string[]): void {
    const { netnsName } = this.config;
    if (netnsName) {
      sudoNetns(netnsName, args);
    } else {
      sudo(args);
    }
  }

  setupNamespace(): void {
    const { guestIp, netnsName } = this.config;
    if (!netnsName) return;
    const slot = this.config.slot;

    const vethHost = `veth-h-${slot}`;
    const vethGuest = `veth-g-${slot}`;
    const transitHostIp = `10.200.${slot}.1`;
    const transitGuestIp = `10.200.${slot}.2`;

    // Clean up stale veth pair if it exists (e.g. from a previous crashed VM)
    if (existsSync(`/sys/class/net/${vethHost}`)) {
      try {
        sudo(["ip", "link", "delete", vethHost]);
      } catch (err) {
        consola.debug(`Stale veth ${vethHost} cleanup failed: ${toError(err).message}`);
      }
    }

    // Create namespace
    sudo(["ip", "netns", "add", netnsName]);

    // Create veth pair
    sudo(["ip", "link", "add", vethHost, "type", "veth", "peer", "name", vethGuest]);

    // Move guest end into namespace
    sudo(["ip", "link", "set", vethGuest, "netns", netnsName]);

    // Configure host end
    sudo(["ip", "addr", "add", `${transitHostIp}/30`, "dev", vethHost]);
    sudo(["ip", "link", "set", vethHost, "up"]);

    // Configure guest end inside namespace
    sudoNetns(netnsName, ["ip", "addr", "add", `${transitGuestIp}/30`, "dev", vethGuest]);
    sudoNetns(netnsName, ["ip", "link", "set", vethGuest, "up"]);
    sudoNetns(netnsName, ["ip", "link", "set", "lo", "up"]);

    // Default route inside namespace via host veth
    sudoNetns(netnsName, ["ip", "route", "add", "default", "via", transitHostIp]);

    // Enable IP forwarding inside namespace
    sudoNetns(netnsName, ["sysctl", "-w", "net.ipv4.ip_forward=1"]);

    // Host: route VM subnet via netns veth
    sudo(["ip", "route", "add", vmLinkCidrFromIp(guestIp), "via", transitGuestIp]);

    // Host: enable IP forwarding
    sudo(["sysctl", "-w", "net.ipv4.ip_forward=1"]);
  }

  teardownNamespace(): void {
    const { guestIp, netnsName } = this.config;
    if (!netnsName) return;

    const tryRun = (args: string[]): void => {
      try {
        sudo(args);
      } catch (err) {
        consola.debug(
          `Namespace teardown command failed (${args.slice(0, 3).join(" ")}): ${toError(err).message}`,
        );
      }
    };

    // Remove host route
    tryRun(["ip", "route", "del", vmLinkCidrFromIp(guestIp)]);

    // Delete namespace — auto-cleans veth pair, TAP device, and rules inside
    tryRun(["ip", "netns", "delete", netnsName]);
  }

  setupDevice(): void {
    const { tapDevice, hostIp, netnsName } = this.config;

    if (!netnsName) {
      // Non-namespaced: cleanup and create TAP in host namespace
      if (existsSync(`/sys/class/net/${tapDevice}`)) {
        try {
          sudo(["ip", "link", "delete", tapDevice]);
        } catch (err) {
          consola.debug(`Stale TAP ${tapDevice} cleanup failed: ${toError(err).message}`);
        }
      }

      sudo(["ip", "tuntap", "add", "dev", tapDevice, "mode", "tap"]);
      sudo(["ip", "addr", "add", `${hostIp}/30`, "dev", tapDevice]);
      sudo(["ip", "link", "set", tapDevice, "up"]);
      sudo(["sysctl", "-w", "net.ipv4.ip_forward=1"]);
    } else {
      // Namespaced: create TAP inside the network namespace
      sudoNetns(netnsName, ["ip", "tuntap", "add", "dev", tapDevice, "mode", "tap"]);
      sudoNetns(netnsName, ["ip", "addr", "add", `${hostIp}/30`, "dev", tapDevice]);
      sudoNetns(netnsName, ["ip", "link", "set", tapDevice, "up"]);
    }
  }

  setupRules(): void {
    const { tapDevice, hostIp, guestIp, publishedPorts, netnsName, slot } = this.config;
    const vmId = resolveVmId(this.config);
    const policy = effectivePolicy(this.config);

    // Build defaultInterface — not needed for deny-all
    let defaultInterface = "";
    if (policy !== "deny-all") {
      defaultInterface = getDefaultInterface();
    }

    const setupConfig = {
      vmId,
      slot,
      policy,
      tapDevice,
      hostIp,
      guestIp,
      vethHost: netnsName ? `veth-h-${slot}` : "",
      vethGuest: netnsName ? `veth-g-${slot}` : "",
      netnsName: netnsName || "",
      defaultInterface,
      publishedPorts: publishedPorts.map((p) => ({
        hostPort: p,
        guestIp,
        guestPort: p,
        protocol: "tcp",
      })),
      allowedCidrs: this.config.allowedCidrs,
      deniedCidrs: this.config.deniedCidrs,
      skipDnat: this.config.skipDnat || false,
      allowIcmp: this.config.allowIcmp ?? false,
      dnsResolvers: DNS_RESOLVERS,
    };

    const result = execNftables("setup", setupConfig);
    if (!result.ok) {
      throw new Error(
        `nftables setup failed: ${result.error || "unknown error"} (code: ${result.code || "none"})`,
      );
    }

    // Start gateway proxies for this VM (non-fatal)
    this.gatewayVmStart(vmId, slot, policy).catch((err) => {
      consola.debug(`Gateway proxy start: ${toError(err).message}`);
    });
  }

  setupThrottle(): void {
    const { tapDevice, bandwidthMbit } = this.config;
    if (bandwidthMbit === undefined) return;
    const rateKbit = bandwidthMbit * 1000;
    const burstKb = Math.max(32, Math.floor(rateKbit / 8));
    this.nsRun([
      "tc",
      "qdisc",
      "add",
      "dev",
      tapDevice,
      "root",
      "tbf",
      "rate",
      `${bandwidthMbit}mbit`,
      "burst",
      `${burstKb}kb`,
      "latency",
      "400ms",
    ]);
  }

  teardownThrottle(): void {
    const { tapDevice } = this.config;
    try {
      this.nsRun(["tc", "qdisc", "del", "dev", tapDevice, "root"]);
    } catch (err) {
      consola.debug(`Throttle teardown for ${tapDevice} failed: ${toError(err).message}`);
    }
  }

  teardownRules(): void {
    const { tapDevice, guestIp, netnsName, slot } = this.config;
    const vmId = resolveVmId(this.config);

    // 0. Stop gateway proxies (best-effort, gateway may not be running)
    this.gatewayVmStop(vmId);

    const teardownResult = execNftables("teardown", {
      vmId,
      netnsName: netnsName || "",
      tapDevice,
      vethHost: netnsName ? `veth-h-${slot}` : "",
      guestIp,
      slot,
    });

    if (!teardownResult.ok) {
      consola.warn(
        `nftables teardown failed for ${vmId}: ${teardownResult.error || "unknown"} (code: ${teardownResult.code || "none"})`,
      );
    }
  }

  teardownDevice(): void {
    const { tapDevice, netnsName } = this.config;
    // When netns is enabled, TAP is auto-cleaned by teardownNamespace()
    if (netnsName) return;
    try {
      sudo(["ip", "link", "delete", tapDevice]);
    } catch (err) {
      consola.debug(`TAP device ${tapDevice} teardown failed: ${toError(err).message}`);
    }
  }

  // ---------------------------------------------------------------------------
  // Gateway proxy helpers (non-fatal — proxies enhance security but are optional)
  // ---------------------------------------------------------------------------

  private _gateway: GatewayClient | null = null;
  private getGateway(): GatewayClient {
    if (!this._gateway) {
      this._gateway = new GatewayClient();
    }
    return this._gateway;
  }

  private async gatewayVmStart(vmId: string, slot: number, policy: string): Promise<void> {
    try {
      await this.getGateway().vmStart({
        vmId,
        slot,
        policy,
        allowedDomains: this.config.allowedDomains,
      });
    } catch (err) {
      consola.debug(`Gateway proxy start: ${toError(err).message}`);
    }
  }

  private gatewayVmStop(vmId: string): void {
    this.getGateway()
      .vmStop(vmId)
      .catch((err) => {
        consola.debug(`Gateway proxy stop: ${toError(err).message}`);
      });
  }

  private gatewayUpdatePolicy(vmId: string, policy: string, allowedDomains?: string[]): void {
    this.getGateway()
      .vmUpdatePolicy(vmId, policy, allowedDomains)
      .catch((err) => {
        consola.debug(`Gateway policy update: ${toError(err).message}`);
      });
  }

  async setup(): Promise<void> {
    this.setupNamespace();
    this.setupDevice();

    // Ensure gateway is running before setting up rules (non-fatal)
    const gatewayBin = join(vmsanPaths().binDir, "vmsan-gateway");
    await ensureGatewayRunning(gatewayBin);

    this.setupRules();
    this.setupThrottle();
  }

  teardown(): void {
    if (this.config.netnsName) {
      // Host-side rules (MASQUERADE, DNAT) need explicit cleanup
      this.teardownRules();
      // Namespace deletion auto-cleans TAP, nftables FORWARD, tc inside
      this.teardownNamespace();
    } else {
      this.teardownThrottle();
      this.teardownRules();
      this.teardownDevice();
    }
  }

  updatePolicy(
    newPolicy: string,
    newDomains: string[],
    newAllowedCidrs: string[],
    newDeniedCidrs: string[],
  ): void {
    // Save old config for rollback
    const oldConfig = { ...this.config };

    this.teardownRules();
    this.config.networkPolicy = newPolicy;
    this.config.allowedDomains = newDomains;
    this.config.allowedCidrs = newAllowedCidrs;
    this.config.deniedCidrs = newDeniedCidrs;

    try {
      this.setupRules();
    } catch (err) {
      // Rollback: restore old config and re-apply old rules (best-effort)
      this.config = oldConfig;
      try {
        this.setupRules();
      } catch (rollbackErr) {
        consola.warn(
          `nftables rollback failed for ${this.config.tapDevice}, VM may have no rules: ${toError(rollbackErr).message}`,
        );
      }
      throw err;
    }

    // Update gateway proxy policy (non-fatal)
    const vmId = resolveVmId(this.config);
    const policy = effectivePolicy(this.config);
    this.gatewayUpdatePolicy(vmId, policy, newDomains);
  }
}

export function verifyCleanup(network: VmNetwork, vmId?: string): string[] {
  const leaks: string[] = [];
  if (existsSync(`/sys/class/net/${network.tapDevice}`)) {
    leaks.push(`TAP device ${network.tapDevice} still exists`);
  }
  const nsExists = network.netnsName && existsSync(`/var/run/netns/${network.netnsName}`);
  if (nsExists) {
    leaks.push(`Namespace ${network.netnsName} still exists`);
  }
  // Only check veth if namespace still exists — when the namespace is deleted,
  // the kernel auto-destroys the veth pair asynchronously
  const slot = slotFromVmHostIpOrNull(network.hostIp);
  if (slot !== null && nsExists) {
    const vethHost = `veth-h-${slot}`;
    if (existsSync(`/sys/class/net/${vethHost}`)) {
      leaks.push(`Veth ${vethHost} still exists`);
    }
  }

  // Check for leaked nftables table (best-effort)
  const resolvedVmId =
    vmId ||
    (network.netnsName?.startsWith("vmsan-") ? network.netnsName.slice("vmsan-".length) : null);
  if (resolvedVmId) {
    try {
      const nftablesPath = join(vmsanPaths().binDir, "vmsan-nftables");
      if (existsSync(nftablesPath)) {
        const verifyResult = execNftables("verify", {
          vmId: resolvedVmId,
          netnsName: network.netnsName || "",
        });
        if (verifyResult.ok && verifyResult.tableExists) {
          leaks.push(`nftables table vmsan_${resolvedVmId} still exists`);
        }
      }
    } catch {
      // Best-effort — skip if binary missing or fails
    }
  }

  return leaks;
}
