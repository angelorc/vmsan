import { describe, it, expect } from "vitest";
import {
  dnsPortForSlot,
  sniPortForSlot,
  httpPortForSlot,
  isReservedPort,
  DNS_PORT_BASE,
  SNI_PORT_BASE,
  HTTP_PORT_BASE,
} from "../../src/lib/network-address.ts";

describe("port allocation constants", () => {
  it("exports DNS_PORT_BASE as 10053", () => {
    expect(DNS_PORT_BASE).toBe(10053);
  });

  it("exports SNI_PORT_BASE as 10443", () => {
    expect(SNI_PORT_BASE).toBe(10443);
  });

  it("exports HTTP_PORT_BASE as 10698", () => {
    expect(HTTP_PORT_BASE).toBe(10698);
  });
});

describe("dnsPortForSlot", () => {
  it("returns 10053 for slot 0", () => {
    expect(dnsPortForSlot(0)).toBe(10053);
  });

  it("returns 10307 for slot 254", () => {
    expect(dnsPortForSlot(254)).toBe(10307);
  });

  it("returns correct port for mid-range slot", () => {
    expect(dnsPortForSlot(100)).toBe(10153);
  });

  it("rejects negative slot", () => {
    expect(() => dnsPortForSlot(-1)).toThrow("invalid VM network slot");
  });

  it("rejects slot 255", () => {
    expect(() => dnsPortForSlot(255)).toThrow("invalid VM network slot");
  });

  it("rejects non-integer slot", () => {
    expect(() => dnsPortForSlot(1.5)).toThrow("invalid VM network slot");
  });
});

describe("sniPortForSlot", () => {
  it("returns 10443 for slot 0", () => {
    expect(sniPortForSlot(0)).toBe(10443);
  });

  it("returns 10697 for slot 254", () => {
    expect(sniPortForSlot(254)).toBe(10697);
  });

  it("returns correct port for mid-range slot", () => {
    expect(sniPortForSlot(100)).toBe(10543);
  });

  it("rejects negative slot", () => {
    expect(() => sniPortForSlot(-1)).toThrow("invalid VM network slot");
  });

  it("rejects slot 255", () => {
    expect(() => sniPortForSlot(255)).toThrow("invalid VM network slot");
  });

  it("rejects non-integer slot", () => {
    expect(() => sniPortForSlot(1.5)).toThrow("invalid VM network slot");
  });
});

describe("httpPortForSlot", () => {
  it("returns 10698 for slot 0", () => {
    expect(httpPortForSlot(0)).toBe(10698);
  });

  it("returns 10952 for slot 254", () => {
    expect(httpPortForSlot(254)).toBe(10952);
  });

  it("returns correct port for mid-range slot", () => {
    expect(httpPortForSlot(100)).toBe(10798);
  });

  it("rejects negative slot", () => {
    expect(() => httpPortForSlot(-1)).toThrow("invalid VM network slot");
  });

  it("rejects slot 255", () => {
    expect(() => httpPortForSlot(255)).toThrow("invalid VM network slot");
  });

  it("rejects non-integer slot", () => {
    expect(() => httpPortForSlot(1.5)).toThrow("invalid VM network slot");
  });
});

describe("isReservedPort", () => {
  it("returns true for DNS_PORT_BASE (first DNS port)", () => {
    expect(isReservedPort(10053)).toBe(true);
  });

  it("returns true for last DNS port (10307)", () => {
    expect(isReservedPort(10307)).toBe(true);
  });

  it("returns true for SNI_PORT_BASE (first SNI port)", () => {
    expect(isReservedPort(10443)).toBe(true);
  });

  it("returns true for last SNI port (10697)", () => {
    expect(isReservedPort(10697)).toBe(true);
  });

  it("returns true for HTTP_PORT_BASE (first HTTP port)", () => {
    expect(isReservedPort(10698)).toBe(true);
  });

  it("returns true for last HTTP port (10952)", () => {
    expect(isReservedPort(10952)).toBe(true);
  });

  it("returns true for mid-range ports in each range", () => {
    expect(isReservedPort(10100)).toBe(true); // DNS range
    expect(isReservedPort(10500)).toBe(true); // SNI range
    expect(isReservedPort(10800)).toBe(true); // HTTP range
  });

  it("returns false for port 80", () => {
    expect(isReservedPort(80)).toBe(false);
  });

  it("returns false for port 443", () => {
    expect(isReservedPort(443)).toBe(false);
  });

  it("returns false for port 8080", () => {
    expect(isReservedPort(8080)).toBe(false);
  });

  it("returns false for port 9999", () => {
    expect(isReservedPort(9999)).toBe(false);
  });

  it("returns false for port just below DNS range", () => {
    expect(isReservedPort(10052)).toBe(false);
  });

  it("returns false for port just above HTTP range", () => {
    // HTTP range ends at 10952 (10698+254), so 10953 is outside all ranges
    expect(isReservedPort(10953)).toBe(false);
  });

  it("returns false for port between DNS and SNI ranges", () => {
    // DNS range ends at 10307, SNI starts at 10443
    // Port 10400 is between DNS and SNI ranges
    expect(isReservedPort(10400)).toBe(false);
  });
});
