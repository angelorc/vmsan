const VM_NETWORK_FIRST_OCTET = 198;
const VM_NETWORK_SECOND_OCTET = 19;

export const VM_SUBNET_MASK: string = "255.255.255.252";
export const VM_NETWORK_PREFIX: string = `${VM_NETWORK_FIRST_OCTET}.${VM_NETWORK_SECOND_OCTET}`;

function assertValidSlot(slot: number): void {
  if (!Number.isInteger(slot) || slot < 0 || slot > 254) {
    throw new Error(`invalid VM network slot: ${slot}`);
  }
}

function parseIpv4(ip: string): [number, number, number, number] {
  const parts = ip.split(".").map((part) => Number(part));
  if (
    parts.length !== 4 ||
    parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)
  ) {
    throw new Error(`invalid IPv4 address: ${ip}`);
  }
  return parts as [number, number, number, number];
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
  const parts = hostIp.split(".").map((part) => Number(part));
  if (
    parts.length !== 4 ||
    parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)
  ) {
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
