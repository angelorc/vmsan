const VM_NETWORK_FIRST_OCTET = 198;
const VM_NETWORK_SECOND_OCTET = 19;

export const VM_SUBNET_MASK: string = "255.255.255.252";
export const VM_NETWORK_PREFIX: string = `${VM_NETWORK_FIRST_OCTET}.${VM_NETWORK_SECOND_OCTET}`;
export const SUPPORTED_VM_ADDRESS_BLOCKS: readonly string[] = ["198.19.0.0/16", "172.16.0.0/16"];

function assertValidSlot(slot: number): void {
  if (!Number.isInteger(slot) || slot < 0 || slot > 254) {
    throw new Error(`invalid VM network slot: ${slot}`);
  }
}

function parseIpv4OrNull(ip: string): [number, number, number, number] | null {
  const octets = ip.split(".");
  if (octets.length !== 4 || octets.some((part) => !/^\d+$/.test(part))) {
    return null;
  }

  const parts = octets.map((part) => Number.parseInt(part, 10));
  if (parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)) {
    return null;
  }

  return parts as [number, number, number, number];
}

function parseIpv4(ip: string): [number, number, number, number] {
  const parts = parseIpv4OrNull(ip);
  if (!parts) {
    throw new Error(`invalid IPv4 address: ${ip}`);
  }
  return parts;
}

export function vmHostIp(slot: number): string {
  assertValidSlot(slot);
  return `${VM_NETWORK_PREFIX}.${slot}.1`;
}

export function vmGuestIp(slot: number): string {
  assertValidSlot(slot);
  return `${VM_NETWORK_PREFIX}.${slot}.2`;
}

export function slotFromVmHostIpOrNull(hostIp: string): number | null {
  const parts = parseIpv4OrNull(hostIp);
  if (!parts) {
    return null;
  }

  const slot = parts[2];
  if (!Number.isInteger(slot) || slot < 0 || slot > 254) {
    return null;
  }

  return slot;
}

export function slotFromVmHostIp(hostIp: string): number {
  const slot = slotFromVmHostIpOrNull(hostIp);
  if (slot === null) {
    throw new Error(`invalid VM host IP: ${hostIp}`);
  }
  return slot;
}

export function vmLinkCidrFromIp(ip: string): string {
  const [first, second, third] = parseIpv4(ip);
  return `${first}.${second}.${third}.0/30`;
}

export function vmAddressBlockCidrFromIp(ip: string): string {
  const [first, second] = parseIpv4(ip);
  return `${first}.${second}.0.0/16`;
}

// Reserved port ranges for future DNS/SNI/HTTP proxies (0.3.0)
export const DNS_PORT_BASE = 10053; // Range: 10053-10307 (255 slots)
export const SNI_PORT_BASE = 10443; // Range: 10443-10697 (255 slots)
export const HTTP_PORT_BASE = 10080; // Range: 10080-10334 (255 slots)

export function dnsPortForSlot(slot: number): number {
  assertValidSlot(slot);
  return DNS_PORT_BASE + slot;
}

export function sniPortForSlot(slot: number): number {
  assertValidSlot(slot);
  return SNI_PORT_BASE + slot;
}

export function httpPortForSlot(slot: number): number {
  assertValidSlot(slot);
  return HTTP_PORT_BASE + slot;
}

/** Check if a port falls within any reserved range. */
export function isReservedPort(port: number): boolean {
  return (
    (port >= DNS_PORT_BASE && port < DNS_PORT_BASE + 255) ||
    (port >= SNI_PORT_BASE && port < SNI_PORT_BASE + 255) ||
    (port >= HTTP_PORT_BASE && port < HTTP_PORT_BASE + 255)
  );
}
