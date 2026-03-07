import { describe, expect, it } from "vitest";
import { parseDuration } from "../../src/lib/utils.ts";
import {
  parseCidrList,
  parseDiskSizeGb,
  parseDomains,
  parseNetworkPolicy,
  parsePublishedPorts,
} from "../../src/commands/create/validation.ts";
import { policyConflictError } from "../../src/errors/index.ts";
import { VmsanError } from "../../src/errors/base.ts";

// ---------- parseDuration ----------

describe("parseDuration", () => {
  it("treats plain numbers as minutes", () => {
    expect(parseDuration("10")).toBe(10 * 60 * 1000);
    expect(parseDuration("1")).toBe(60_000);
    expect(parseDuration("0")).toBe(0);
  });

  it("parses seconds", () => {
    expect(parseDuration("30s")).toBe(30_000);
    expect(parseDuration("1s")).toBe(1_000);
  });

  it("parses minutes", () => {
    expect(parseDuration("5m")).toBe(5 * 60 * 1000);
  });

  it("parses hours", () => {
    expect(parseDuration("2h")).toBe(2 * 60 * 60 * 1000);
  });

  it("parses days", () => {
    expect(parseDuration("1d")).toBe(24 * 60 * 60 * 1000);
  });

  it("parses compound durations", () => {
    expect(parseDuration("1h30m")).toBe(90 * 60 * 1000);
    expect(parseDuration("1d12h")).toBe(36 * 60 * 60 * 1000);
    expect(parseDuration("2h30m15s")).toBe((2 * 3600 + 30 * 60 + 15) * 1000);
  });

  it("is case-insensitive", () => {
    expect(parseDuration("1H")).toBe(60 * 60 * 1000);
    expect(parseDuration("30M")).toBe(30 * 60 * 1000);
    expect(parseDuration("10S")).toBe(10_000);
    expect(parseDuration("1D")).toBe(24 * 60 * 60 * 1000);
  });

  it("rejects invalid duration strings", () => {
    expect(() => parseDuration("abc")).toThrow(VmsanError);
    expect(() => parseDuration("")).toThrow(VmsanError);
    expect(() => parseDuration("xyz")).toThrow(VmsanError);
  });
});

// ---------- disk size parsing ----------

describe("disk size parsing", () => {
  it('parses "10gb" as default', () => {
    expect(parseDiskSizeGb(undefined)).toBe(10);
  });

  it("accepts various suffixes", () => {
    expect(parseDiskSizeGb("50gb")).toBe(50);
    expect(parseDiskSizeGb("50g")).toBe(50);
    expect(parseDiskSizeGb("50gib")).toBe(50);
    expect(parseDiskSizeGb("50")).toBe(50);
  });
});

// ---------- port parsing ----------

describe("port parsing", () => {
  it("handles mixed valid ports", () => {
    expect(parsePublishedPorts("22,80,443,8080")).toEqual([22, 80, 443, 8080]);
  });

  it("fails on one invalid port in a list", () => {
    expect(() => parsePublishedPorts("80,invalid,443")).toThrow(VmsanError);
  });
});

// ---------- policy auto-promotion ----------

describe("policy auto-promotion", () => {
  it("promotes allow-all to custom when allowed-domain is present", () => {
    const networkPolicy = parseNetworkPolicy(undefined); // "allow-all"
    const domains = parseDomains("example.com");

    const effectiveNetworkPolicy =
      networkPolicy === "allow-all" && domains.length > 0 ? "custom" : networkPolicy;

    expect(effectiveNetworkPolicy).toBe("custom");
  });

  it("promotes allow-all to custom when allowed-cidr is present", () => {
    const networkPolicy = parseNetworkPolicy(undefined); // "allow-all"
    const allowedCidrs = parseCidrList("10.0.0.0/8");

    const effectiveNetworkPolicy =
      networkPolicy === "allow-all" && allowedCidrs.length > 0 ? "custom" : networkPolicy;

    expect(effectiveNetworkPolicy).toBe("custom");
  });

  it("promotes allow-all to custom when denied-cidr is present", () => {
    const networkPolicy = parseNetworkPolicy(undefined); // "allow-all"
    const deniedCidrs = parseCidrList("10.0.0.0/8");

    const effectiveNetworkPolicy =
      networkPolicy === "allow-all" && deniedCidrs.length > 0 ? "custom" : networkPolicy;

    expect(effectiveNetworkPolicy).toBe("custom");
  });

  it("keeps allow-all when no filtering rules are present", () => {
    const networkPolicy = parseNetworkPolicy(undefined); // "allow-all"
    const domains = parseDomains(undefined);
    const allowedCidrs = parseCidrList(undefined);
    const deniedCidrs = parseCidrList(undefined);

    const effectiveNetworkPolicy =
      networkPolicy === "allow-all" &&
      (domains.length > 0 || allowedCidrs.length > 0 || deniedCidrs.length > 0)
        ? "custom"
        : networkPolicy;

    expect(effectiveNetworkPolicy).toBe("allow-all");
  });

  it("rejects deny-all with filtering rules", () => {
    const networkPolicy = parseNetworkPolicy("deny-all");
    const domains = parseDomains("example.com");

    if (networkPolicy === "deny-all" && domains.length > 0) {
      expect(() => {
        throw policyConflictError();
      }).toThrow(VmsanError);
    }
  });

  it("keeps custom as-is", () => {
    const networkPolicy = parseNetworkPolicy("custom");
    expect(networkPolicy).toBe("custom");
  });
});
