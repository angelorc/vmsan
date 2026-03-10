import { describe, expect, it, vi } from "vitest";
import {
  parseBandwidth,
  parseCidrList,
  parseDiskSizeGb,
  parseDomains,
  parseImageReference,
  parseMemoryMib,
  parseNetworkPolicy,
  parsePublishedPorts,
  parseRuntime,
  parseVcpuCount,
  validateCidr,
  validatePublishedPortsAvailable,
} from "../../src/commands/create/validation.ts";
import { VmsanError } from "../../src/errors/base.ts";
import type { VmsanPaths } from "../../src/paths.ts";

vi.mock("node:fs", async (importOriginal) => {
  const actual = await importOriginal<typeof import("node:fs")>();
  return {
    ...actual,
    readdirSync: vi.fn().mockReturnValue([]),
    existsSync: vi.fn().mockReturnValue(false),
    readFileSync: vi.fn(),
    mkdirSync: vi.fn(),
  };
});

// ---------- vcpus ----------

describe("parseVcpuCount", () => {
  it("defaults to 1 when no value is provided", () => {
    expect(parseVcpuCount(undefined)).toBe(1);
  });

  it("parses valid vcpu counts", () => {
    expect(parseVcpuCount("1")).toBe(1);
    expect(parseVcpuCount("4")).toBe(4);
    expect(parseVcpuCount("32")).toBe(32);
  });

  it("rejects values below minimum (1)", () => {
    expect(() => parseVcpuCount("0")).toThrow(VmsanError);
    expect(() => parseVcpuCount("-1")).toThrow(VmsanError);
  });

  it("rejects values above maximum (32)", () => {
    expect(() => parseVcpuCount("33")).toThrow(VmsanError);
    expect(() => parseVcpuCount("64")).toThrow(VmsanError);
  });

  it("rejects non-integer values", () => {
    expect(() => parseVcpuCount("1.5")).toThrow(VmsanError);
    expect(() => parseVcpuCount("abc")).toThrow(VmsanError);
    expect(() => parseVcpuCount("")).toThrow(VmsanError);
  });

  it("includes flag name in error message", () => {
    try {
      parseVcpuCount("0");
    } catch (e) {
      expect((e as VmsanError).message).toContain("--vcpus");
    }
  });
});

// ---------- memory ----------

describe("parseMemoryMib", () => {
  it("defaults to 128 when no value is provided", () => {
    expect(parseMemoryMib(undefined)).toBe(128);
  });

  it("parses valid memory values", () => {
    expect(parseMemoryMib("128")).toBe(128);
    expect(parseMemoryMib("256")).toBe(256);
    expect(parseMemoryMib("32768")).toBe(32768);
  });

  it("rejects values below minimum (128)", () => {
    expect(() => parseMemoryMib("0")).toThrow(VmsanError);
    expect(() => parseMemoryMib("127")).toThrow(VmsanError);
  });

  it("rejects values above maximum (32768)", () => {
    expect(() => parseMemoryMib("32769")).toThrow(VmsanError);
  });

  it("rejects non-integer values", () => {
    expect(() => parseMemoryMib("128.5")).toThrow(VmsanError);
    expect(() => parseMemoryMib("abc")).toThrow(VmsanError);
  });

  it("includes MiB unit suffix in error message", () => {
    try {
      parseMemoryMib("0");
    } catch (e) {
      expect((e as VmsanError).message).toContain("MiB");
    }
  });
});

// ---------- runtime ----------

describe("parseRuntime", () => {
  it('defaults to "base" when no value is provided', () => {
    expect(parseRuntime(undefined)).toBe("base");
    expect(parseRuntime("")).toBe("base");
  });

  it("accepts all valid runtimes", () => {
    expect(parseRuntime("base")).toBe("base");
    expect(parseRuntime("node22")).toBe("node22");
    expect(parseRuntime("node24")).toBe("node24");
    expect(parseRuntime("python3.13")).toBe("python3.13");
  });

  it("rejects invalid runtimes", () => {
    expect(() => parseRuntime("go")).toThrow(VmsanError);
    expect(() => parseRuntime("ruby")).toThrow(VmsanError);
  });

  it("includes valid options in error message", () => {
    try {
      parseRuntime("go");
    } catch (e) {
      expect((e as VmsanError).message).toContain("base");
      expect((e as VmsanError).message).toContain("node22");
    }
  });
});

// ---------- network-policy ----------

describe("parseNetworkPolicy", () => {
  it('defaults to "allow-all" when no value is provided', () => {
    expect(parseNetworkPolicy(undefined)).toBe("allow-all");
    expect(parseNetworkPolicy("")).toBe("allow-all");
  });

  it("accepts all valid policies", () => {
    expect(parseNetworkPolicy("allow-all")).toBe("allow-all");
    expect(parseNetworkPolicy("deny-all")).toBe("deny-all");
    expect(parseNetworkPolicy("custom")).toBe("custom");
  });

  it("rejects invalid policies", () => {
    expect(() => parseNetworkPolicy("block")).toThrow(VmsanError);
    expect(() => parseNetworkPolicy("none")).toThrow(VmsanError);
  });
});

// ---------- ports ----------

describe("parsePublishedPorts", () => {
  it("returns empty array when no value is provided", () => {
    expect(parsePublishedPorts(undefined)).toEqual([]);
    expect(parsePublishedPorts("")).toEqual([]);
  });

  it("parses single port", () => {
    expect(parsePublishedPorts("80")).toEqual([80]);
  });

  it("parses comma-separated ports", () => {
    expect(parsePublishedPorts("80,443,8080")).toEqual([80, 443, 8080]);
  });

  it("trims whitespace around ports", () => {
    expect(parsePublishedPorts(" 80 , 443 ")).toEqual([80, 443]);
  });

  it("accepts boundary values", () => {
    expect(parsePublishedPorts("1")).toEqual([1]);
    expect(parsePublishedPorts("65535")).toEqual([65535]);
  });

  it("rejects port 0", () => {
    expect(() => parsePublishedPorts("0")).toThrow(VmsanError);
  });

  it("rejects ports above 65535", () => {
    expect(() => parsePublishedPorts("65536")).toThrow(VmsanError);
  });

  it("rejects non-numeric ports", () => {
    expect(() => parsePublishedPorts("http")).toThrow(VmsanError);
  });

  it("rejects negative ports", () => {
    expect(() => parsePublishedPorts("-1")).toThrow(VmsanError);
  });
});

// ---------- domains ----------

describe("parseDomains", () => {
  it("returns empty array when no value is provided", () => {
    expect(parseDomains(undefined)).toEqual([]);
    expect(parseDomains("")).toEqual([]);
  });

  it("parses single domain", () => {
    expect(parseDomains("example.com")).toEqual(["example.com"]);
  });

  it("parses comma-separated domains", () => {
    expect(parseDomains("a.com,b.org")).toEqual(["a.com", "b.org"]);
  });

  it("normalizes domains to lowercase", () => {
    expect(parseDomains("Example.COM")).toEqual(["example.com"]);
  });

  it("accepts wildcard domain with leading *. prefix", () => {
    expect(parseDomains("*.example.com")).toEqual(["*.example.com"]);
  });

  it("rejects wildcard in non-leading position", () => {
    expect(() => parseDomains("example.*.com")).toThrow(VmsanError);
  });

  it("rejects multiple wildcards", () => {
    expect(() => parseDomains("*.*.example.com")).toThrow(VmsanError);
  });

  it("rejects domains with whitespace", () => {
    expect(() => parseDomains("exam ple.com")).toThrow(VmsanError);
  });

  it("rejects domains with newlines", () => {
    expect(() => parseDomains("example.com\nmalicious.com")).toThrow(VmsanError);
  });

  it("rejects domains longer than 253 characters", () => {
    const veryLong = `${"a".repeat(250)}.com`;
    expect(() => parseDomains(veryLong)).toThrow(VmsanError);
  });

  it("rejects domains with invalid label characters", () => {
    expect(() => parseDomains("exam_ple.com")).toThrow(VmsanError);
  });
});

// ---------- CIDRs ----------

describe("parseCidrList", () => {
  it("returns empty array when no value is provided", () => {
    expect(parseCidrList(undefined)).toEqual([]);
    expect(parseCidrList("")).toEqual([]);
  });

  it("parses comma-separated CIDRs", () => {
    expect(parseCidrList("10.0.0.0/8,192.168.1.0/24")).toEqual(["10.0.0.0/8", "192.168.1.0/24"]);
  });

  it("trims whitespace", () => {
    expect(parseCidrList(" 10.0.0.0/8 ")).toEqual(["10.0.0.0/8"]);
  });
});

describe("validateCidr", () => {
  it("accepts valid CIDRs", () => {
    expect(() => validateCidr("10.0.0.0/8")).not.toThrow();
    expect(() => validateCidr("192.168.1.0/24")).not.toThrow();
    expect(() => validateCidr("0.0.0.0/0")).not.toThrow();
    expect(() => validateCidr("255.255.255.255/32")).not.toThrow();
  });

  it("rejects malformed format", () => {
    expect(() => validateCidr("10.0.0.0")).toThrow(VmsanError);
    expect(() => validateCidr("not-cidr")).toThrow(VmsanError);
    expect(() => validateCidr("10.0.0/24")).toThrow(VmsanError);
  });

  it("rejects prefix > 32", () => {
    expect(() => validateCidr("10.0.0.0/33")).toThrow(VmsanError);
  });

  it("rejects octets > 255", () => {
    expect(() => validateCidr("256.0.0.0/24")).toThrow(VmsanError);
    expect(() => validateCidr("10.0.0.256/24")).toThrow(VmsanError);
  });

  it("error message mentions CIDR format", () => {
    try {
      validateCidr("invalid");
    } catch (e) {
      expect((e as VmsanError).message).toContain("CIDR");
    }
  });
});

// ---------- image-ref ----------

describe("parseImageReference", () => {
  it("parses image with explicit tag", () => {
    const ref = parseImageReference("ubuntu:22.04");
    expect(ref.name).toBe("ubuntu");
    expect(ref.tag).toBe("22.04");
    expect(ref.full).toBe("ubuntu:22.04");
  });

  it("defaults to latest tag when none is given", () => {
    const ref = parseImageReference("ubuntu");
    expect(ref.tag).toBe("latest");
    expect(ref.full).toBe("ubuntu:latest");
  });

  it("handles registry prefix", () => {
    const ref = parseImageReference("docker.io/library/ubuntu:22.04");
    expect(ref.name).toBe("docker.io/library/ubuntu");
    expect(ref.tag).toBe("22.04");
  });

  it("generates cacheKey replacing colons with slashes", () => {
    const ref = parseImageReference("docker.io/library/ubuntu:22.04");
    expect(ref.cacheKey).toBe("docker.io/library/ubuntu/22.04");
  });

  it("rejects empty image reference", () => {
    expect(() => parseImageReference("")).toThrow(VmsanError);
    expect(() => parseImageReference("   ")).toThrow(VmsanError);
  });

  it("rejects image reference with empty tag (trailing colon)", () => {
    expect(() => parseImageReference("ubuntu:")).toThrow(VmsanError);
  });
});

// ---------- disk size ----------

describe("parseDiskSizeGb", () => {
  it("defaults to 10gb when no value is provided", () => {
    expect(parseDiskSizeGb(undefined)).toBe(10);
  });

  it("parses values with gb suffix", () => {
    expect(parseDiskSizeGb("20gb")).toBe(20);
    expect(parseDiskSizeGb("20GB")).toBe(20);
  });

  it("parses values with g suffix", () => {
    expect(parseDiskSizeGb("20g")).toBe(20);
  });

  it("parses values with gib suffix", () => {
    expect(parseDiskSizeGb("20gib")).toBe(20);
  });

  it("parses bare numbers", () => {
    expect(parseDiskSizeGb("20")).toBe(20);
  });

  it("accepts boundary values", () => {
    expect(parseDiskSizeGb("1gb")).toBe(1);
    expect(parseDiskSizeGb("1024gb")).toBe(1024);
  });

  it("rejects 0gb", () => {
    expect(() => parseDiskSizeGb("0gb")).toThrow(VmsanError);
  });

  it("rejects above 1024gb", () => {
    expect(() => parseDiskSizeGb("1025gb")).toThrow(VmsanError);
  });

  it("rejects non-numeric input", () => {
    expect(() => parseDiskSizeGb("abc")).toThrow(VmsanError);
  });
});

// ---------- bandwidth ----------

describe("parseBandwidth", () => {
  it("returns undefined when no value is provided", () => {
    expect(parseBandwidth(undefined)).toBeUndefined();
  });

  it("parses bare number", () => {
    expect(parseBandwidth("100")).toBe(100);
  });

  it("parses value with mbit suffix", () => {
    expect(parseBandwidth("100mbit")).toBe(100);
    expect(parseBandwidth("100m")).toBe(100);
    expect(parseBandwidth("100M")).toBe(100);
  });

  it("accepts boundary values", () => {
    expect(parseBandwidth("1")).toBe(1);
    expect(parseBandwidth("1000")).toBe(1000);
  });

  it("rejects 0", () => {
    expect(() => parseBandwidth("0")).toThrow(VmsanError);
  });

  it("rejects above 1000", () => {
    expect(() => parseBandwidth("1001")).toThrow(VmsanError);
  });

  it("rejects non-numeric input", () => {
    expect(() => parseBandwidth("fast")).toThrow(VmsanError);
  });
});

// ---------- validatePublishedPortsAvailable ----------

describe("validatePublishedPortsAvailable", () => {
  const fakePaths: VmsanPaths = {
    baseDir: "/fake/.vmsan",
    vmsDir: "/fake/.vmsan/vms",
    jailerBaseDir: "/fake/.vmsan/jailer",
    binDir: "/fake/.vmsan/bin",
    agentBin: "/fake/.vmsan/bin/vmsan-agent",
    nftablesBin: "/fake/.vmsan/bin/vmsan-nftables",
    kernelsDir: "/fake/.vmsan/kernels",
    rootfsDir: "/fake/.vmsan/rootfs",
    registryDir: "/fake/.vmsan/registry/rootfs",
    snapshotsDir: "/fake/.vmsan/snapshots",
    seccompDir: "/fake/.vmsan/seccomp",
    seccompFilter: "/fake/.vmsan/seccomp/default.json",
    agentPort: 9119,
  };

  it("rejects port 10053 (DNS_PORT_BASE) as reserved", () => {
    expect(() => validatePublishedPortsAvailable([10053], fakePaths)).toThrow(VmsanError);
    expect(() => validatePublishedPortsAvailable([10053], fakePaths)).toThrow("reserved");
  });

  it("rejects port 10307 (last DNS port) as reserved", () => {
    expect(() => validatePublishedPortsAvailable([10307], fakePaths)).toThrow(VmsanError);
    expect(() => validatePublishedPortsAvailable([10307], fakePaths)).toThrow("reserved");
  });

  it("rejects port 10443 (SNI_PORT_BASE) as reserved", () => {
    expect(() => validatePublishedPortsAvailable([10443], fakePaths)).toThrow(VmsanError);
    expect(() => validatePublishedPortsAvailable([10443], fakePaths)).toThrow("reserved");
  });

  it("rejects port 10080 (HTTP_PORT_BASE) as reserved", () => {
    expect(() => validatePublishedPortsAvailable([10080], fakePaths)).toThrow(VmsanError);
    expect(() => validatePublishedPortsAvailable([10080], fakePaths)).toThrow("reserved");
  });

  it("accepts port 80 (not reserved)", () => {
    expect(() => validatePublishedPortsAvailable([80], fakePaths)).not.toThrow();
  });

  it("accepts port 443 (not reserved)", () => {
    expect(() => validatePublishedPortsAvailable([443], fakePaths)).not.toThrow();
  });

  it("accepts port 9999 (not reserved)", () => {
    expect(() => validatePublishedPortsAvailable([9999], fakePaths)).not.toThrow();
  });

  it("does nothing for empty ports array", () => {
    expect(() => validatePublishedPortsAvailable([], fakePaths)).not.toThrow();
  });
});
