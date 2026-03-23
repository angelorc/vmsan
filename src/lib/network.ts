import type { VmNetwork } from "./vm-state.ts";
import {
  slotFromVmHostIp,
  vmGuestIp,
  vmHostIp,
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
  project?: string;
  service?: string;
  connectTo?: string[];
}

// DNS resolvers the VM uses (Google Public DNS)
const DNS_RESOLVERS = ["8.8.8.8", "8.8.4.4"];

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
      project?: string;
      service?: string;
      connectTo?: string[];
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
      service: network.service,
      connectTo: network.connectTo,
    });
  }

}
