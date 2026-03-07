import { describe, expect, it } from "vitest";
import {
  VM_NETWORK_PREFIX,
  VM_SUBNET_MASK,
  SUPPORTED_VM_ADDRESS_BLOCKS,
  slotFromVmHostIp,
  slotFromVmHostIpOrNull,
  vmAddressBlockCidrFromIp,
  vmGuestIp,
  vmHostIp,
  vmLinkCidrFromIp,
} from "../../src/lib/network-address.ts";

describe("network-address", () => {
  // ---------- constants ----------

  it("exports expected constants", () => {
    expect(VM_NETWORK_PREFIX).toBe("198.19");
    expect(VM_SUBNET_MASK).toBe("255.255.255.252");
    expect(SUPPORTED_VM_ADDRESS_BLOCKS).toContain("198.19.0.0/16");
    expect(SUPPORTED_VM_ADDRESS_BLOCKS).toContain("172.16.0.0/16");
  });

  // ---------- vmHostIp / vmGuestIp ----------

  it("builds the default VM host and guest addresses in the benchmark range", () => {
    expect(vmHostIp(7)).toBe("198.19.7.1");
    expect(vmGuestIp(7)).toBe("198.19.7.2");
  });

  it("builds addresses for slot 0 (first slot)", () => {
    expect(vmHostIp(0)).toBe("198.19.0.1");
    expect(vmGuestIp(0)).toBe("198.19.0.2");
  });

  it("builds addresses for slot 254 (last slot)", () => {
    expect(vmHostIp(254)).toBe("198.19.254.1");
    expect(vmGuestIp(254)).toBe("198.19.254.2");
  });

  it("builds addresses for mid-range slot", () => {
    expect(vmHostIp(128)).toBe("198.19.128.1");
    expect(vmGuestIp(128)).toBe("198.19.128.2");
  });

  it("rejects negative slot numbers", () => {
    expect(() => vmHostIp(-1)).toThrow("invalid VM network slot");
    expect(() => vmGuestIp(-1)).toThrow("invalid VM network slot");
  });

  it("rejects slot 255 (out of range)", () => {
    expect(() => vmHostIp(255)).toThrow("invalid VM network slot");
    expect(() => vmGuestIp(255)).toThrow("invalid VM network slot");
  });

  it("rejects non-integer slot numbers", () => {
    expect(() => vmHostIp(1.5)).toThrow("invalid VM network slot");
    expect(() => vmGuestIp(1.5)).toThrow("invalid VM network slot");
  });

  // ---------- slotFromVmHostIp ----------

  it("derives the VM slot from persisted host address", () => {
    expect(slotFromVmHostIp("198.19.42.1")).toBe(42);
  });

  it("derives slot 0 from host IP", () => {
    expect(slotFromVmHostIp("198.19.0.1")).toBe(0);
  });

  it("derives slot 254 from host IP", () => {
    expect(slotFromVmHostIp("198.19.254.1")).toBe(254);
  });

  it("throws for invalid host IP", () => {
    expect(() => slotFromVmHostIp("invalid")).toThrow("invalid VM host IP");
  });

  // ---------- slotFromVmHostIpOrNull ----------

  it("returns null for malformed host IPs", () => {
    expect(slotFromVmHostIpOrNull("invalid")).toBeNull();
    expect(slotFromVmHostIpOrNull("198.19..1")).toBeNull();
    expect(slotFromVmHostIpOrNull("198.19.1e2.1")).toBeNull();
  });

  it("returns null for slot 255 (out of range)", () => {
    expect(slotFromVmHostIpOrNull("198.19.255.1")).toBeNull();
  });

  it("returns null for slot 999 (invalid octet)", () => {
    expect(slotFromVmHostIpOrNull("198.19.999.1")).toBeNull();
  });

  it("returns valid slot for in-range IPs", () => {
    expect(slotFromVmHostIpOrNull("198.19.0.1")).toBe(0);
    expect(slotFromVmHostIpOrNull("198.19.254.1")).toBe(254);
    expect(slotFromVmHostIpOrNull("198.19.100.1")).toBe(100);
  });

  // ---------- vmLinkCidrFromIp ----------

  it("derives link CIDR from IP", () => {
    expect(vmLinkCidrFromIp("198.19.42.2")).toBe("198.19.42.0/30");
    expect(vmLinkCidrFromIp("198.19.0.1")).toBe("198.19.0.0/30");
    expect(vmLinkCidrFromIp("198.19.254.2")).toBe("198.19.254.0/30");
  });

  // ---------- vmAddressBlockCidrFromIp ----------

  it("derives address block CIDR from IP", () => {
    expect(vmAddressBlockCidrFromIp("198.19.42.2")).toBe("198.19.0.0/16");
    expect(vmAddressBlockCidrFromIp("10.20.30.40")).toBe("10.20.0.0/16");
  });

  // ---------- legacy address compatibility ----------

  it("preserves compatibility with legacy persisted 172.16 addresses", () => {
    expect(slotFromVmHostIp("172.16.9.1")).toBe(9);
    expect(vmLinkCidrFromIp("172.16.9.2")).toBe("172.16.9.0/30");
    expect(vmAddressBlockCidrFromIp("172.16.9.2")).toBe("172.16.0.0/16");
  });

  // ---------- round-trip ----------

  it("round-trips slot -> host IP -> slot", () => {
    for (const slot of [0, 1, 127, 200, 254]) {
      const ip = vmHostIp(slot);
      expect(slotFromVmHostIp(ip)).toBe(slot);
    }
  });
});
