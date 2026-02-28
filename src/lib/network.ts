import { execSync } from "node:child_process";
import { existsSync } from "node:fs";
import { defaultInterfaceNotFoundError } from "../errors/index.ts";

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
}

function run(cmd: string): void {
  execSync(cmd, { stdio: "pipe" });
}

function getDefaultInterface(): string {
  const output = execSync("ip route show default 2>/dev/null | awk '{print $5}' | head -1", {
    encoding: "utf-8",
  }).trim();
  if (!output) {
    throw defaultInterfaceNotFoundError();
  }
  return output;
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
  readonly config: NetworkConfig;

  constructor(
    slot: number,
    networkPolicy: string,
    allowedDomains: string[],
    allowedCidrs: string[],
    deniedCidrs: string[],
    publishedPorts: number[],
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
    };
  }

  static bootArgs(slot: number): string {
    const hostIp = `172.16.${slot}.1`;
    return `console=ttyS0 reboot=k panic=1 pci=off ip=172.16.${slot}.2::${hostIp}:255.255.255.252::eth0:off:${hostIp}`;
  }

  static fromConfig(config: NetworkConfig): NetworkManager {
    const mgr = Object.create(NetworkManager.prototype) as NetworkManager;
    (mgr as any).config = config;
    return mgr;
  }

  async setup(): Promise<void> {
    const { tapDevice, hostIp, guestIp, publishedPorts } = this.config;

    // Best-effort cleanup for leaked TAP devices from interrupted runs.
    if (existsSync(`/sys/class/net/${tapDevice}`)) {
      try {
        run(`sudo ip link delete ${tapDevice}`);
      } catch {
        // Leaked TAP device may be in-use; creation will fail with clear error
      }
    }

    // Create TAP device, assign IP, bring up
    run(`sudo ip tuntap add dev ${tapDevice} mode tap`);
    run(`sudo ip addr add ${hostIp}/30 dev ${tapDevice}`);
    run(`sudo ip link set ${tapDevice} up`);

    // Enable IP forwarding
    run("sudo sysctl -w net.ipv4.ip_forward=1");

    const policy = effectivePolicy(this.config);

    if (policy === "deny-all") {
      // DROP all FORWARD traffic on this TAP
      run(`sudo iptables -I FORWARD -i ${tapDevice} -j DROP`);
      run(`sudo iptables -I FORWARD -o ${tapDevice} -j DROP`);
      return;
    }

    const defaultIface = getDefaultInterface();

    // MASQUERADE for outbound NAT
    run(`sudo iptables -t nat -A POSTROUTING -s ${guestIp}/30 -o ${defaultIface} -j MASQUERADE`);

    if (policy === "custom") {
      // 1. Denied CIDRs
      for (const cidr of this.config.deniedCidrs) {
        run(`sudo iptables -A FORWARD -i ${tapDevice} -d ${cidr} -j DROP`);
      }

      // 2. Allow DNS to host CoreDNS only (must come before cross-VM isolation)
      run(`sudo iptables -A FORWARD -i ${tapDevice} -d ${hostIp} -p udp --dport 53 -j ACCEPT`);
      run(`sudo iptables -A FORWARD -i ${tapDevice} -d ${hostIp} -p tcp --dport 53 -j ACCEPT`);

      // 3. Block external DNS (prevent DNS bypass)
      run(`sudo iptables -A FORWARD -i ${tapDevice} -p udp --dport 53 -j DROP`);
      run(`sudo iptables -A FORWARD -i ${tapDevice} -p tcp --dport 53 -j DROP`);
      run(`sudo iptables -A FORWARD -i ${tapDevice} -p udp --dport 853 -j DROP`);
      run(`sudo iptables -A FORWARD -i ${tapDevice} -p tcp --dport 853 -j DROP`);

      // 4. Cross-VM isolation: block traffic to other VM subnets
      run(`sudo iptables -A FORWARD -i ${tapDevice} -d 172.16.0.0/16 -j DROP`);

      // 5. Allowed CIDRs (bypass DNS gatekeeper)
      for (const cidr of this.config.allowedCidrs) {
        run(`sudo iptables -A FORWARD -i ${tapDevice} -d ${cidr} -j ACCEPT`);
      }

      // 6. ACCEPT all remaining (DNS gatekeeper controls domain access)
      run(`sudo iptables -A FORWARD -i ${tapDevice} -j ACCEPT`);
    } else {
      // allow-all mode: unrestricted, but still protect DNS bypass and VM isolation

      // DNS bypass protection
      run(`sudo iptables -A FORWARD -i ${tapDevice} -d ${hostIp} -p udp --dport 53 -j ACCEPT`);
      run(`sudo iptables -A FORWARD -i ${tapDevice} -d ${hostIp} -p tcp --dport 53 -j ACCEPT`);
      run(`sudo iptables -A FORWARD -i ${tapDevice} -p udp --dport 53 -j DROP`);
      run(`sudo iptables -A FORWARD -i ${tapDevice} -p tcp --dport 53 -j DROP`);
      run(`sudo iptables -A FORWARD -i ${tapDevice} -p udp --dport 853 -j DROP`);
      run(`sudo iptables -A FORWARD -i ${tapDevice} -p tcp --dport 853 -j DROP`);

      // Cross-VM isolation
      run(`sudo iptables -A FORWARD -i ${tapDevice} -d 172.16.0.0/16 -j DROP`);

      // Allow all other FORWARD traffic
      run(`sudo iptables -A FORWARD -i ${tapDevice} -j ACCEPT`);
    }

    // Return traffic
    run(`sudo iptables -A FORWARD -o ${tapDevice} -m state --state RELATED,ESTABLISHED -j ACCEPT`);

    // Port forwarding: DNAT rules (scoped to external interface)
    for (const port of publishedPorts) {
      run(
        `sudo iptables -t nat -A PREROUTING -i ${defaultIface} -p tcp --dport ${port} -j DNAT --to-destination ${guestIp}:${port}`,
      );
      run(`sudo iptables -A FORWARD -p tcp -d ${guestIp} --dport ${port} -j ACCEPT`);
    }
  }

  teardown(): void {
    const { tapDevice, hostIp, guestIp, publishedPorts } = this.config;

    let defaultIface: string | undefined;
    try {
      defaultIface = getDefaultInterface();
    } catch {
      // Interface may not exist; cleanup proceeds without NAT rule removal
    }

    const tryRun = (cmd: string): void => {
      try {
        run(cmd);
      } catch {
        // Best-effort iptables cleanup — rule may already be removed
      }
    };

    // Remove port forwarding rules
    for (const port of publishedPorts) {
      if (defaultIface) {
        tryRun(
          `sudo iptables -t nat -D PREROUTING -i ${defaultIface} -p tcp --dport ${port} -j DNAT --to-destination ${guestIp}:${port}`,
        );
      }
      tryRun(`sudo iptables -D FORWARD -p tcp -d ${guestIp} --dport ${port} -j ACCEPT`);
    }

    // Remove denied CIDR rules
    for (const cidr of this.config.deniedCidrs) {
      tryRun(`sudo iptables -D FORWARD -i ${tapDevice} -d ${cidr} -j DROP`);
    }

    // Remove allowed CIDR rules
    for (const cidr of this.config.allowedCidrs) {
      tryRun(`sudo iptables -D FORWARD -i ${tapDevice} -d ${cidr} -j ACCEPT`);
    }

    // Remove DNS rules (host-specific + external block)
    tryRun(`sudo iptables -D FORWARD -i ${tapDevice} -d ${hostIp} -p udp --dport 53 -j ACCEPT`);
    tryRun(`sudo iptables -D FORWARD -i ${tapDevice} -d ${hostIp} -p tcp --dport 53 -j ACCEPT`);
    tryRun(`sudo iptables -D FORWARD -i ${tapDevice} -p udp --dport 53 -j DROP`);
    tryRun(`sudo iptables -D FORWARD -i ${tapDevice} -p tcp --dport 53 -j DROP`);
    tryRun(`sudo iptables -D FORWARD -i ${tapDevice} -p udp --dport 853 -j DROP`);
    tryRun(`sudo iptables -D FORWARD -i ${tapDevice} -p tcp --dport 853 -j DROP`);

    // Remove cross-VM isolation rule
    tryRun(`sudo iptables -D FORWARD -i ${tapDevice} -d 172.16.0.0/16 -j DROP`);

    // Remove FORWARD rules
    tryRun(`sudo iptables -D FORWARD -i ${tapDevice} -j ACCEPT`);
    tryRun(
      `sudo iptables -D FORWARD -o ${tapDevice} -m state --state RELATED,ESTABLISHED -j ACCEPT`,
    );
    tryRun(`sudo iptables -D FORWARD -i ${tapDevice} -j DROP`);
    tryRun(`sudo iptables -D FORWARD -o ${tapDevice} -j DROP`);

    // Remove MASQUERADE
    if (defaultIface) {
      tryRun(
        `sudo iptables -t nat -D POSTROUTING -s ${guestIp}/30 -o ${defaultIface} -j MASQUERADE`,
      );
    }

    // Delete TAP device
    tryRun(`sudo ip link delete ${tapDevice}`);
  }
}
