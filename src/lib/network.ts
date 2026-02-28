import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { defaultInterfaceNotFoundError } from "../errors/index.ts";
import type { VmNetwork } from "./vm-state.ts";

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
}

// Well-known DoH/DoQ resolver IPs (Google, Cloudflare, Quad9, OpenDNS, CleanBrowsing)
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
  ) {
    this.config = {
      slot,
      tapDevice: `fhvm${slot}`,
      hostIp: `172.16.${slot}.1`,
      guestIp: `172.16.${slot}.2`,
      subnetMask: "255.255.255.252",
      macAddress: `AA:FC:00:00:00:${(slot + 1).toString(16).padStart(2, "0").toUpperCase()}`,
      networkPolicy,
      allowedDomains,
      allowedCidrs,
      deniedCidrs,
      publishedPorts,
      bandwidthMbit,
      netnsName,
    };
  }

  static bootArgs(slot: number): string {
    const hostIp = `172.16.${slot}.1`;
    return `console=ttyS0 reboot=k panic=1 pci=off ip=172.16.${slot}.2::${hostIp}:255.255.255.252::eth0:off:${hostIp}`;
  }

  static fromConfig(config: NetworkConfig): NetworkManager {
    const mgr = Object.create(NetworkManager.prototype) as NetworkManager;
    mgr.config = config;
    return mgr;
  }

  static fromVmNetwork(network: VmNetwork): NetworkManager {
    const slot = Number(network.hostIp.split(".")[2]);
    if (!Number.isInteger(slot)) {
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
    const { slot, netnsName } = this.config;
    if (!netnsName) return;

    const vethHost = `veth-h-${slot}`;
    const vethGuest = `veth-g-${slot}`;
    const transitHostIp = `10.200.${slot}.1`;
    const transitGuestIp = `10.200.${slot}.2`;

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
    sudo(["ip", "route", "add", `172.16.${slot}.0/30`, "via", transitGuestIp]);

    // Host: enable IP forwarding
    sudo(["sysctl", "-w", "net.ipv4.ip_forward=1"]);
  }

  teardownNamespace(): void {
    const { slot, netnsName } = this.config;
    if (!netnsName) return;

    const tryRun = (args: string[]): void => {
      try {
        sudo(args);
      } catch {
        // Best-effort cleanup
      }
    };

    // Remove host route
    tryRun(["ip", "route", "del", `172.16.${slot}.0/30`]);

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
        } catch {
          // Leaked TAP device may be in-use; creation will fail with clear error
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
    const { tapDevice, hostIp, guestIp, publishedPorts } = this.config;
    const policy = effectivePolicy(this.config);
    // FORWARD/filtering rules go inside netns when enabled; NAT/DNAT stay on host
    const fwd = this.nsRun.bind(this);

    if (policy === "deny-all") {
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

    if (policy === "custom") {
      // 1. Denied CIDRs
      for (const cidr of this.config.deniedCidrs) {
        fwd(["iptables", "-A", "FORWARD", "-i", tapDevice, "-d", cidr, "-j", "DROP"]);
      }

      // 2. Allow DNS to host CoreDNS only
      fwd([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        tapDevice,
        "-d",
        hostIp,
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
        hostIp,
        "-p",
        "tcp",
        "--dport",
        "53",
        "-j",
        "ACCEPT",
      ]);

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
      fwd(["iptables", "-A", "FORWARD", "-i", tapDevice, "-d", "172.16.0.0/16", "-j", "DROP"]);

      // 6. Allowed CIDRs
      for (const cidr of this.config.allowedCidrs) {
        fwd(["iptables", "-A", "FORWARD", "-i", tapDevice, "-d", cidr, "-j", "ACCEPT"]);
      }

      // 7. ACCEPT all remaining
      fwd(["iptables", "-A", "FORWARD", "-i", tapDevice, "-j", "ACCEPT"]);
    } else {
      // allow-all mode
      fwd([
        "iptables",
        "-A",
        "FORWARD",
        "-i",
        tapDevice,
        "-d",
        hostIp,
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
        hostIp,
        "-p",
        "tcp",
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

      fwd(["iptables", "-A", "FORWARD", "-i", tapDevice, "-d", "172.16.0.0/16", "-j", "DROP"]);
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

    // Port forwarding: DNAT rules (always on host — external traffic arrives there)
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
    } catch {
      // Best-effort — qdisc may not exist
    }
  }

  teardownRules(): void {
    const { tapDevice, hostIp, guestIp, publishedPorts, netnsName } = this.config;

    let defaultIface: string | undefined;
    try {
      defaultIface = getDefaultInterface();
    } catch {
      // Interface may not exist; cleanup proceeds without NAT rule removal
    }

    const tryRun = (args: string[]): void => {
      try {
        sudo(args);
      } catch {
        // Best-effort iptables cleanup — rule may already be removed
      }
    };

    const tryFwd = (args: string[]): void => {
      try {
        this.nsRun(args);
      } catch {
        // Best-effort cleanup
      }
    };

    // Remove port forwarding rules (always on host)
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

    // When netns is enabled, FORWARD rules inside the namespace are auto-cleaned
    // by teardownNamespace(). Only clean host-side rules here.
    if (!netnsName) {
      for (const cidr of this.config.deniedCidrs) {
        tryFwd(["iptables", "-D", "FORWARD", "-i", tapDevice, "-d", cidr, "-j", "DROP"]);
      }
      for (const cidr of this.config.allowedCidrs) {
        tryFwd(["iptables", "-D", "FORWARD", "-i", tapDevice, "-d", cidr, "-j", "ACCEPT"]);
      }

      tryFwd([
        "iptables",
        "-D",
        "FORWARD",
        "-i",
        tapDevice,
        "-d",
        hostIp,
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
        hostIp,
        "-p",
        "tcp",
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

      tryFwd(["iptables", "-D", "FORWARD", "-i", tapDevice, "-d", "172.16.0.0/16", "-j", "DROP"]);
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
    } catch {
      // Best-effort — TAP may already be deleted
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
      } catch {
        // Rollback failed — VM has no rules; caller should handle
      }
      throw err;
    }
  }
}
