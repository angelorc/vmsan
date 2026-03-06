import { describe, expect, it } from "vitest";
import {
  slotFromVmHostIp,
  vmAddressBlockCidrFromIp,
  vmGuestIp,
  vmHostIp,
  vmLinkCidrFromIp,
} from "../../src/lib/network-address.ts";

describe("network-address", () => {
  it("builds the default VM host and guest addresses in the benchmark range", () => {
    expect(vmHostIp(7)).toBe("198.19.7.1");
    expect(vmGuestIp(7)).toBe("198.19.7.2");
  });

  it("derives the VM slot and cidrs from persisted host and guest addresses", () => {
    expect(slotFromVmHostIp("198.19.42.1")).toBe(42);
    expect(vmLinkCidrFromIp("198.19.42.2")).toBe("198.19.42.0/30");
    expect(vmAddressBlockCidrFromIp("198.19.42.2")).toBe("198.19.0.0/16");
  });

  it("preserves compatibility with legacy persisted 172.16 addresses", () => {
    expect(slotFromVmHostIp("172.16.9.1")).toBe(9);
    expect(vmLinkCidrFromIp("172.16.9.2")).toBe("172.16.9.0/30");
    expect(vmAddressBlockCidrFromIp("172.16.9.2")).toBe("172.16.0.0/16");
  });
});
