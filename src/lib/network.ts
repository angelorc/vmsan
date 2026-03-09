import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { consola } from "consola";
import { defaultInterfaceNotFoundError } from "../errors/index.ts";
import { vmsanPaths } from "../paths.ts";
import { toError } from "./utils.ts";
import type { VmNetwork } from "./vm-state.ts";
import {
  slotFromVmHostIp,
  slotFromVmHostIpOrNull,
  vmGuestIp,
  vmHostIp,
  vmLinkCidrFromIp,
  SUPPORTED_VM_ADDRESS_BLOCKS,
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
  /** VM identifier, used by nftables backend for per-VM table naming. */
  vmId?: string;
}

// DNS resolvers the VM uses (Google Public DNS)
const DNS_RESOLVERS = ["8.8.8.8", "8.8.4.4"];

// Well-known DoH/DoQ resolver IPs — only used by legacy iptables path
// (nftables binary handles DoH/DoT blocking internally)
const DOH_RESOLVER_IPS = [
  "8.8.8.8",
  "8.8.4.4",
  "1.1.1.1",
  "1.0.0.1",
  "9.9.9.9",
  "149.112.112.112",
  "208.67.222.222",
  "208.67.220.220",
  "185.228.168.168",
  "185.228.169.168",
];

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
// iptables helpers (shared by legacy path and non-rule methods)
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

  constructor(
    slot: number,
    networkPolicy: string,
    allowedDomains: string[],
    allowedCidrs: string[],
    deniedCidrs: string[],
    publishedPorts: number[],
    bandwidthMbit?: number,
    netnsName?: string,
    skipDnat?: boolean,
  ) {
    this.config = {
      slot,
      tapDevice: `fhvm${slot}`,
      hostIp: vmHostIp(slot),
      guestIp: vmGuestIp(slot),
      subnetMask: VM_SUBNET_MASK,
      macAddress: `AA:FC:00:00:00:${(slot + 1).toString(16).padStart(2, "0").toUpperCase()}`,
      networkPolicy,
      allowedDomains,
      allowedCidrs,
      deniedCidrs,
      publishedPorts,
      bandwidthMbit,
      netnsName,
      skipDnat,
    };
  }

  static bootArgs(config: Pick<NetworkConfig, "guestIp" | "hostIp" | "subnetMask">): string {
    return `console=ttyS0 reboot=k panic=1 pci=off ip=${config.guestIp}::${config.hostIp}:${config.subnetMask}::eth0:off:${DNS_RESOLVERS[0]}`;
  }

  static fromConfig(config: NetworkConfig): NetworkManager {
    const mgr = Object.create(NetworkManager.prototype) as NetworkManager;
    mgr.config = config;
    return mgr;
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

    // Delete namespace — auto-cleans veth pair, TAP device, and iptables inside
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
    if (process.env.VMSAN_LEGACY_IPTABLES === "1") {
      this.setupRulesIptables();
      return;
    }

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
      dnsResolvers: DNS_RESOLVERS,
    };

    const result = execNftables("setup", setupConfig);
    if (!result.ok) {
      throw new Error(
        `nftables setup failed: ${result.error || "unknown error"} (code: ${result.code || "none"})`,
      );
    }

    // Host-side iptables FORWARD + MASQUERADE — required because nftables
    // chains are independent and cannot bypass Docker's iptables-nft FORWARD
    // DROP policy. These rules go into the shared iptables FORWARD chain.
    if (policy !== "deny-all") {
      const fwdDevice = netnsName ? `veth-h-${slot}` : tapDevice;
      sudo([
        "iptables",
        "-t",
        "nat",
        "-A",
        "POSTROUTING",
        "-s",
        `${guestIp}/30`,
        "-o",
        defaultInterface,
        "-j",
        "MASQUERADE",
      ]);
      sudo([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        fwdDevice,
        "-o",
        defaultInterface,
        "-s",
        `${guestIp}/30`,
        "-j",
        "ACCEPT",
      ]);
      sudo([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        defaultInterface,
        "-o",
        fwdDevice,
        "-d",
        `${guestIp}/30`,
        "-m",
        "state",
        "--state",
        "RELATED,ESTABLISHED",
        "-j",
        "ACCEPT",
      ]);
    }

    // Host-side DNAT + FORWARD accept for published ports
    if (!this.config.skipDnat && policy !== "deny-all") {
      for (const port of publishedPorts) {
        const portStr = String(port);
        sudo([
          "iptables",
          "-t",
          "nat",
          "-A",
          "PREROUTING",
          "-i",
          defaultInterface,
          "-p",
          "tcp",
          "--dport",
          portStr,
          "-j",
          "DNAT",
          "--to-destination",
          `${guestIp}:${portStr}`,
        ]);
        sudo([
          "iptables",
          "-A",
          "FORWARD",
          "-p",
          "tcp",
          "-d",
          guestIp,
          "--dport",
          portStr,
          "-j",
          "ACCEPT",
        ]);
      }
    }
  }

  /** Legacy iptables-based setupRules, gated behind VMSAN_LEGACY_IPTABLES=1 */
  private setupRulesIptables(): void {
    const { tapDevice, guestIp, publishedPorts } = this.config;
    const policy = effectivePolicy(this.config);
    const vethGuest = this.config.netnsName ? `veth-g-${this.config.slot}` : undefined;
    // FORWARD/filtering rules go inside netns when enabled; NAT/DNAT stay on host
    const fwd = this.nsRun.bind(this);

    // Host-originated traffic to the guest agent and services must bypass
    // host firewalls that deny special-purpose/private ranges by default.
    sudo(["iptables", "-I", "OUTPUT", "1", "-d", guestIp, "-j", "ACCEPT"]);
    sudo(["iptables", "-I", "INPUT", "1", "-s", guestIp, "-j", "ACCEPT"]);

    if (policy === "deny-all") {
      if (vethGuest) {
        // Host-originated traffic to the guest must not depend on the namespace
        // FORWARD default policy. This keeps agent/PTy/tunnel access consistent
        // across hosts with different iptables defaults.
        fwd([
          "iptables",
          "-I",
          "FORWARD",
          "1",
          "-i",
          vethGuest,
          "-o",
          tapDevice,
          "-d",
          guestIp,
          "-j",
          "ACCEPT",
        ]);
      }
      fwd(["iptables", "-I", "FORWARD", "-i", tapDevice, "-j", "DROP"]);
      fwd(["iptables", "-I", "FORWARD", "-o", tapDevice, "-j", "DROP"]);
      return;
    }

    const defaultIface = getDefaultInterface();

    // MASQUERADE for outbound NAT (always on host)
    sudo([
      "iptables",
      "-t",
      "nat",
      "-A",
      "POSTROUTING",
      "-s",
      `${guestIp}/30`,
      "-o",
      defaultIface,
      "-j",
      "MASQUERADE",
    ]);

    // When netns is enabled, traffic exits via veth pair — host needs FORWARD rules
    if (this.config.netnsName) {
      const vethHost = `veth-h-${this.config.slot}`;
      sudo([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        vethHost,
        "-o",
        defaultIface,
        "-s",
        `${guestIp}/30`,
        "-j",
        "ACCEPT",
      ]);
      sudo([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        defaultIface,
        "-o",
        vethHost,
        "-d",
        `${guestIp}/30`,
        "-m",
        "state",
        "--state",
        "RELATED,ESTABLISHED",
        "-j",
        "ACCEPT",
      ]);
    }

    if (policy === "custom") {
      // 1. Denied CIDRs
      for (const cidr of this.config.deniedCidrs) {
        fwd(["iptables", "-A", "FORWARD", "-i", tapDevice, "-d", cidr, "-j", "DROP"]);
      }

      // 2. Allow DNS to configured resolvers only
      for (const dnsIp of DNS_RESOLVERS) {
        fwd([
          "iptables",
          "-A",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          dnsIp,
          "-p",
          "udp",
          "--dport",
          "53",
          "-j",
          "ACCEPT",
        ]);
        fwd([
          "iptables",
          "-A",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          dnsIp,
          "-p",
          "tcp",
          "--dport",
          "53",
          "-j",
          "ACCEPT",
        ]);
      }

      // 3. Block external DNS
      fwd([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "udp",
        "--dport",
        "53",
        "-j",
        "DROP",
      ]);
      fwd([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "tcp",
        "--dport",
        "53",
        "-j",
        "DROP",
      ]);
      fwd([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "udp",
        "--dport",
        "853",
        "-j",
        "DROP",
      ]);
      fwd([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "tcp",
        "--dport",
        "853",
        "-j",
        "DROP",
      ]);

      // 4. Block DoH/DoQ to well-known resolvers
      for (const ip of DOH_RESOLVER_IPS) {
        fwd([
          "iptables",
          "-A",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          ip,
          "-p",
          "tcp",
          "--dport",
          "443",
          "-j",
          "DROP",
        ]);
        fwd([
          "iptables",
          "-A",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          ip,
          "-p",
          "udp",
          "--dport",
          "443",
          "-j",
          "DROP",
        ]);
      }

      // 5. Cross-VM isolation
      for (const vmAddressBlock of SUPPORTED_VM_ADDRESS_BLOCKS) {
        fwd(["iptables", "-A", "FORWARD", "-i", tapDevice, "-d", vmAddressBlock, "-j", "DROP"]);
      }

      // 6. Allowed CIDRs
      for (const cidr of this.config.allowedCidrs) {
        fwd(["iptables", "-A", "FORWARD", "-i", tapDevice, "-d", cidr, "-j", "ACCEPT"]);
      }

      // 7. ACCEPT all remaining
      fwd(["iptables", "-A", "FORWARD", "-i", tapDevice, "-j", "ACCEPT"]);
    } else {
      // allow-all mode
      for (const dnsIp of DNS_RESOLVERS) {
        fwd([
          "iptables",
          "-A",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          dnsIp,
          "-p",
          "udp",
          "--dport",
          "53",
          "-j",
          "ACCEPT",
        ]);
        fwd([
          "iptables",
          "-A",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          dnsIp,
          "-p",
          "tcp",
          "--dport",
          "53",
          "-j",
          "ACCEPT",
        ]);
      }
      fwd([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "udp",
        "--dport",
        "53",
        "-j",
        "DROP",
      ]);
      fwd([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "tcp",
        "--dport",
        "53",
        "-j",
        "DROP",
      ]);
      fwd([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "udp",
        "--dport",
        "853",
        "-j",
        "DROP",
      ]);
      fwd([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "tcp",
        "--dport",
        "853",
        "-j",
        "DROP",
      ]);

      for (const ip of DOH_RESOLVER_IPS) {
        fwd([
          "iptables",
          "-A",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          ip,
          "-p",
          "tcp",
          "--dport",
          "443",
          "-j",
          "DROP",
        ]);
        fwd([
          "iptables",
          "-A",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          ip,
          "-p",
          "udp",
          "--dport",
          "443",
          "-j",
          "DROP",
        ]);
      }

      for (const vmAddressBlock of SUPPORTED_VM_ADDRESS_BLOCKS) {
        fwd(["iptables", "-A", "FORWARD", "-i", tapDevice, "-d", vmAddressBlock, "-j", "DROP"]);
      }
      fwd(["iptables", "-A", "FORWARD", "-i", tapDevice, "-j", "ACCEPT"]);
    }

    // Return traffic
    fwd([
      "iptables",
      "-A",
      "FORWARD",
      "-o",
      tapDevice,
      "-m",
      "state",
      "--state",
      "RELATED,ESTABLISHED",
      "-j",
      "ACCEPT",
    ]);

    // Port forwarding: DNAT rules (skipped when Cloudflare Tunnel handles routing)
    if (!this.config.skipDnat) {
      for (const port of publishedPorts) {
        const portStr = String(port);
        sudo([
          "iptables",
          "-t",
          "nat",
          "-A",
          "PREROUTING",
          "-i",
          defaultIface,
          "-p",
          "tcp",
          "--dport",
          portStr,
          "-j",
          "DNAT",
          "--to-destination",
          `${guestIp}:${portStr}`,
        ]);
        sudo([
          "iptables",
          "-A",
          "FORWARD",
          "-p",
          "tcp",
          "-d",
          guestIp,
          "--dport",
          portStr,
          "-j",
          "ACCEPT",
        ]);
      }
    }

    if (vethGuest) {
      // Explicitly allow host namespace -> guest namespace traffic. Without
      // this, agent reachability depends on the netns FORWARD default policy.
      fwd([
        "iptables",
        "-I",
        "FORWARD",
        "1",
        "-i",
        vethGuest,
        "-o",
        tapDevice,
        "-d",
        guestIp,
        "-j",
        "ACCEPT",
      ]);
    }
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
    if (process.env.VMSAN_LEGACY_IPTABLES === "1") {
      this.teardownRulesIptables();
      return;
    }

    const { tapDevice, hostIp, guestIp, netnsName, slot } = this.config;
    const vmId = resolveVmId(this.config);

    // 1. Try nftables teardown first (namespace table + host bypass chains)
    const teardownResult = execNftables("teardown", {
      vmId,
      netnsName: netnsName || "",
    });

    if (!teardownResult.ok) {
      // 2. If nftables table not found, try cleanup-iptables as fallback (0.1.0 VMs)
      if (teardownResult.code === "NFTABLES_ERROR") {
        consola.debug(
          `nftables teardown returned NFTABLES_ERROR for ${vmId}, trying iptables cleanup fallback`,
        );
        const cleanupResult = execNftables("cleanup-iptables", {
          vmId,
          tapDevice,
          vethHost: netnsName ? `veth-h-${slot}` : "",
          vethGuest: netnsName ? `veth-g-${slot}` : "",
          netnsName: netnsName || "",
          hostIp,
          guestIp,
        });
        if (!cleanupResult.ok) {
          consola.warn(
            `iptables cleanup fallback failed for ${vmId}: ${cleanupResult.error || "unknown"} (code: ${cleanupResult.code || "none"})`,
          );
        }
      } else {
        // 3. Other errors — log warning, don't throw
        consola.warn(
          `nftables teardown failed for ${vmId}: ${teardownResult.error || "unknown"} (code: ${teardownResult.code || "none"})`,
        );
      }
    }

    // 4. Clean up host-side iptables FORWARD + MASQUERADE (best-effort)
    this.teardownHostIptablesRules();
  }

  /** Remove host-side iptables FORWARD, MASQUERADE, and DNAT rules (best-effort). */
  private teardownHostIptablesRules(): void {
    const { tapDevice, guestIp, publishedPorts, netnsName, slot } = this.config;

    let defaultIface: string | undefined;
    try {
      defaultIface = getDefaultInterface();
    } catch (err) {
      consola.debug(`Default interface detection skipped during teardown: ${toError(err).message}`);
      return;
    }

    const fwdDevice = netnsName ? `veth-h-${slot}` : tapDevice;

    const tryRun = (args: string[]): void => {
      try {
        sudo(args);
      } catch (err) {
        consola.debug(
          `Host iptables cleanup (${args.slice(0, 4).join(" ")}): ${toError(err).message}`,
        );
      }
    };

    // FORWARD rules
    tryRun([
      "iptables",
      "-D",
      "FORWARD",
      "-i",
      fwdDevice,
      "-o",
      defaultIface,
      "-s",
      `${guestIp}/30`,
      "-j",
      "ACCEPT",
    ]);
    tryRun([
      "iptables",
      "-D",
      "FORWARD",
      "-i",
      defaultIface,
      "-o",
      fwdDevice,
      "-d",
      `${guestIp}/30`,
      "-m",
      "state",
      "--state",
      "RELATED,ESTABLISHED",
      "-j",
      "ACCEPT",
    ]);

    // MASQUERADE
    tryRun([
      "iptables",
      "-t",
      "nat",
      "-D",
      "POSTROUTING",
      "-s",
      `${guestIp}/30`,
      "-o",
      defaultIface,
      "-j",
      "MASQUERADE",
    ]);

    // Published port DNAT + FORWARD
    if (!this.config.skipDnat) {
      for (const port of publishedPorts) {
        const portStr = String(port);
        tryRun([
          "iptables",
          "-t",
          "nat",
          "-D",
          "PREROUTING",
          "-i",
          defaultIface,
          "-p",
          "tcp",
          "--dport",
          portStr,
          "-j",
          "DNAT",
          "--to-destination",
          `${guestIp}:${portStr}`,
        ]);
        tryRun([
          "iptables",
          "-D",
          "FORWARD",
          "-p",
          "tcp",
          "-d",
          guestIp,
          "--dport",
          portStr,
          "-j",
          "ACCEPT",
        ]);
      }
    }
  }

  /** Legacy iptables-based teardownRules, gated behind VMSAN_LEGACY_IPTABLES=1 */
  private teardownRulesIptables(): void {
    const { tapDevice, guestIp, publishedPorts, netnsName } = this.config;
    const vethGuest = netnsName ? `veth-g-${this.config.slot}` : undefined;

    let defaultIface: string | undefined;
    try {
      defaultIface = getDefaultInterface();
    } catch (err) {
      consola.debug(`Default interface detection skipped: ${toError(err).message}`);
    }

    const tryRun = (args: string[]): void => {
      try {
        sudo(args);
      } catch (err) {
        consola.debug(
          `iptables host cleanup failed (${args.slice(0, 4).join(" ")}): ${toError(err).message}`,
        );
      }
    };

    tryRun(["iptables", "-D", "OUTPUT", "-d", guestIp, "-j", "ACCEPT"]);
    tryRun(["iptables", "-D", "INPUT", "-s", guestIp, "-j", "ACCEPT"]);

    const tryFwd = (args: string[]): void => {
      try {
        this.nsRun(args);
      } catch (err) {
        consola.debug(
          `iptables fwd cleanup failed (${args.slice(0, 4).join(" ")}): ${toError(err).message}`,
        );
      }
    };

    // Remove port forwarding rules (skipped when DNAT was not created)
    if (!this.config.skipDnat) {
      for (const port of publishedPorts) {
        const portStr = String(port);
        if (defaultIface) {
          tryRun([
            "iptables",
            "-t",
            "nat",
            "-D",
            "PREROUTING",
            "-i",
            defaultIface,
            "-p",
            "tcp",
            "--dport",
            portStr,
            "-j",
            "DNAT",
            "--to-destination",
            `${guestIp}:${portStr}`,
          ]);
        }
        tryRun([
          "iptables",
          "-D",
          "FORWARD",
          "-p",
          "tcp",
          "-d",
          guestIp,
          "--dport",
          portStr,
          "-j",
          "ACCEPT",
        ]);
      }
    }

    // When netns is enabled, FORWARD rules inside the namespace are auto-cleaned
    // by teardownNamespace(). Only clean host-side rules here.
    if (!netnsName) {
      for (const cidr of this.config.deniedCidrs) {
        tryFwd(["iptables", "-D", "FORWARD", "-i", tapDevice, "-d", cidr, "-j", "DROP"]);
      }
      for (const cidr of this.config.allowedCidrs) {
        tryFwd(["iptables", "-D", "FORWARD", "-i", tapDevice, "-d", cidr, "-j", "ACCEPT"]);
      }

      for (const dnsIp of DNS_RESOLVERS) {
        tryFwd([
          "iptables",
          "-D",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          dnsIp,
          "-p",
          "udp",
          "--dport",
          "53",
          "-j",
          "ACCEPT",
        ]);
        tryFwd([
          "iptables",
          "-D",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          dnsIp,
          "-p",
          "tcp",
          "--dport",
          "53",
          "-j",
          "ACCEPT",
        ]);
      }
      tryFwd([
        "iptables",
        "-D",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "udp",
        "--dport",
        "53",
        "-j",
        "DROP",
      ]);
      tryFwd([
        "iptables",
        "-D",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "tcp",
        "--dport",
        "53",
        "-j",
        "DROP",
      ]);
      tryFwd([
        "iptables",
        "-D",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "udp",
        "--dport",
        "853",
        "-j",
        "DROP",
      ]);
      tryFwd([
        "iptables",
        "-D",
        "FORWARD",
        "-i",
        tapDevice,
        "-p",
        "tcp",
        "--dport",
        "853",
        "-j",
        "DROP",
      ]);

      for (const ip of DOH_RESOLVER_IPS) {
        tryFwd([
          "iptables",
          "-D",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          ip,
          "-p",
          "tcp",
          "--dport",
          "443",
          "-j",
          "DROP",
        ]);
        tryFwd([
          "iptables",
          "-D",
          "FORWARD",
          "-i",
          tapDevice,
          "-d",
          ip,
          "-p",
          "udp",
          "--dport",
          "443",
          "-j",
          "DROP",
        ]);
      }

      for (const vmAddressBlock of SUPPORTED_VM_ADDRESS_BLOCKS) {
        tryFwd(["iptables", "-D", "FORWARD", "-i", tapDevice, "-d", vmAddressBlock, "-j", "DROP"]);
      }
      tryFwd(["iptables", "-D", "FORWARD", "-i", tapDevice, "-j", "ACCEPT"]);
      tryFwd([
        "iptables",
        "-D",
        "FORWARD",
        "-o",
        tapDevice,
        "-m",
        "state",
        "--state",
        "RELATED,ESTABLISHED",
        "-j",
        "ACCEPT",
      ]);
      tryFwd(["iptables", "-D", "FORWARD", "-i", tapDevice, "-j", "DROP"]);
      tryFwd(["iptables", "-D", "FORWARD", "-o", tapDevice, "-j", "DROP"]);
    }

    if (vethGuest) {
      tryFwd([
        "iptables",
        "-D",
        "FORWARD",
        "-i",
        vethGuest,
        "-o",
        tapDevice,
        "-d",
        guestIp,
        "-j",
        "ACCEPT",
      ]);
    }

    // Remove host-side veth FORWARD rules (netns mode)
    if (netnsName && defaultIface) {
      const vethHost = `veth-h-${this.config.slot}`;
      tryRun([
        "iptables",
        "-D",
        "FORWARD",
        "-i",
        vethHost,
        "-o",
        defaultIface,
        "-s",
        `${guestIp}/30`,
        "-j",
        "ACCEPT",
      ]);
      tryRun([
        "iptables",
        "-D",
        "FORWARD",
        "-i",
        defaultIface,
        "-o",
        vethHost,
        "-d",
        `${guestIp}/30`,
        "-m",
        "state",
        "--state",
        "RELATED,ESTABLISHED",
        "-j",
        "ACCEPT",
      ]);
    }

    // Remove MASQUERADE (always on host)
    if (defaultIface) {
      tryRun([
        "iptables",
        "-t",
        "nat",
        "-D",
        "POSTROUTING",
        "-s",
        `${guestIp}/30`,
        "-o",
        defaultIface,
        "-j",
        "MASQUERADE",
      ]);
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

  async setup(): Promise<void> {
    this.setupNamespace();
    this.setupDevice();
    this.setupRules();
    this.setupThrottle();
  }

  teardown(): void {
    if (this.config.netnsName) {
      // Host-side rules (MASQUERADE, DNAT) need explicit cleanup
      this.teardownRules();
      // Namespace deletion auto-cleans TAP, iptables FORWARD, tc inside
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
