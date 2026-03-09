import { describe, test, expect, vi, beforeEach } from "vitest";
import { existsSync } from "node:fs";
import { execFileSync } from "node:child_process";
import type { VmNetwork } from "../../src/lib/vm-state.ts";

vi.mock("node:fs", async (importOriginal) => {
  const actual = await importOriginal<typeof import("node:fs")>();
  return {
    ...actual,
    existsSync: vi.fn(),
  };
});

vi.mock("node:child_process", async (importOriginal) => {
  const actual = await importOriginal<typeof import("node:child_process")>();
  return {
    ...actual,
    execFileSync: vi.fn(),
  };
});

vi.mock("../../src/paths.ts", () => ({
  vmsanPaths: () => ({
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
  }),
}));

import { verifyCleanup } from "../../src/lib/network.ts";

function makeNetwork(overrides?: Partial<VmNetwork>): VmNetwork {
  return {
    tapDevice: "fhvm0",
    hostIp: "198.19.0.1",
    guestIp: "198.19.0.2",
    subnetMask: "255.255.255.252",
    macAddress: "AA:FC:00:00:00:01",
    networkPolicy: "allow-all",
    allowedDomains: [],
    allowedCidrs: [],
    deniedCidrs: [],
    publishedPorts: [],
    tunnelHostname: null,
    netnsName: "vmsan-test",
    ...overrides,
  };
}

describe("verifyCleanup", () => {
  beforeEach(() => {
    vi.mocked(existsSync).mockReset();
    vi.mocked(execFileSync).mockReset();
  });

  test("detects orphaned TAP device", () => {
    vi.mocked(existsSync).mockImplementation((p) => {
      if (String(p) === "/sys/class/net/fhvm0") return true;
      return false;
    });

    const leaks = verifyCleanup(makeNetwork());
    expect(leaks).toContain("TAP device fhvm0 still exists");
  });

  test("returns empty array for clean state", () => {
    vi.mocked(existsSync).mockReturnValue(false);

    const leaks = verifyCleanup(makeNetwork());
    expect(leaks).toEqual([]);
  });

  test("detects orphaned namespace", () => {
    vi.mocked(existsSync).mockImplementation((p) => {
      if (String(p) === "/var/run/netns/vmsan-test") return true;
      return false;
    });

    const leaks = verifyCleanup(makeNetwork());
    expect(leaks).toContain("Namespace vmsan-test still exists");
  });

  test("detects orphaned veth when namespace also still exists", () => {
    vi.mocked(existsSync).mockImplementation((p) => {
      const s = String(p);
      if (s === "/var/run/netns/vmsan-test") return true;
      if (s === "/sys/class/net/veth-h-0") return true;
      return false;
    });

    const leaks = verifyCleanup(makeNetwork());
    expect(leaks).toContain("Namespace vmsan-test still exists");
    expect(leaks).toContain("Veth veth-h-0 still exists");
  });

  test("skips veth check when namespace is already gone (kernel auto-cleans)", () => {
    vi.mocked(existsSync).mockImplementation((p) => {
      // Namespace gone, but veth-h still briefly visible (kernel race)
      if (String(p) === "/sys/class/net/veth-h-0") return true;
      return false;
    });

    const leaks = verifyCleanup(makeNetwork());
    expect(leaks).not.toContainEqual(expect.stringContaining("Veth"));
  });

  test("skips veth check when netnsName is absent", () => {
    vi.mocked(existsSync).mockReturnValue(true);

    const leaks = verifyCleanup(makeNetwork({ netnsName: undefined }));
    // Should report TAP but not namespace or veth
    expect(leaks).toContain("TAP device fhvm0 still exists");
    expect(leaks).not.toContainEqual(expect.stringContaining("Veth"));
    expect(leaks).not.toContainEqual(expect.stringContaining("Namespace"));
  });

  // ---------- nftables table leak detection ----------

  test("detects nftables table leak when verify returns tableExists:true", () => {
    vi.mocked(existsSync).mockImplementation((p) => {
      const s = String(p);
      // Binary exists
      if (s === "/fake/.vmsan/bin/vmsan-nftables") return true;
      return false;
    });

    vi.mocked(execFileSync).mockReturnValue(
      JSON.stringify({ ok: true, tableExists: true }) as unknown as Buffer,
    );

    const leaks = verifyCleanup(makeNetwork(), "my-vm");
    expect(leaks).toContain("nftables table vmsan_my-vm still exists");
  });

  test("skips nftables check when binary is missing", () => {
    vi.mocked(existsSync).mockReturnValue(false);

    const leaks = verifyCleanup(makeNetwork(), "my-vm");
    // No error thrown, no nftables leak reported
    expect(leaks).not.toContainEqual(expect.stringContaining("nftables"));
  });

  test("no nftables leak when verify returns tableExists:false", () => {
    vi.mocked(existsSync).mockImplementation((p) => {
      const s = String(p);
      if (s === "/fake/.vmsan/bin/vmsan-nftables") return true;
      return false;
    });

    vi.mocked(execFileSync).mockReturnValue(
      JSON.stringify({ ok: true, tableExists: false }) as unknown as Buffer,
    );

    const leaks = verifyCleanup(makeNetwork(), "my-vm");
    expect(leaks).not.toContainEqual(expect.stringContaining("nftables"));
  });

  test("includes nftables table leak alongside other leaks", () => {
    vi.mocked(existsSync).mockImplementation((p) => {
      const s = String(p);
      if (s === "/sys/class/net/fhvm0") return true; // TAP leak
      if (s === "/fake/.vmsan/bin/vmsan-nftables") return true; // binary exists
      return false;
    });

    vi.mocked(execFileSync).mockReturnValue(
      JSON.stringify({ ok: true, tableExists: true }) as unknown as Buffer,
    );

    const leaks = verifyCleanup(makeNetwork(), "my-vm");
    expect(leaks).toContain("TAP device fhvm0 still exists");
    expect(leaks).toContain("nftables table vmsan_my-vm still exists");
    expect(leaks).toHaveLength(2);
  });

  test("resolves vmId from netnsName when vmId not provided", () => {
    vi.mocked(existsSync).mockImplementation((p) => {
      const s = String(p);
      if (s === "/fake/.vmsan/bin/vmsan-nftables") return true;
      return false;
    });

    vi.mocked(execFileSync).mockReturnValue(
      JSON.stringify({ ok: true, tableExists: true }) as unknown as Buffer,
    );

    // netnsName is "vmsan-test" -> resolves to vmId "test"
    const leaks = verifyCleanup(makeNetwork());
    expect(leaks).toContain("nftables table vmsan_test still exists");
  });

  test("silently skips nftables check when execFileSync throws", () => {
    vi.mocked(existsSync).mockImplementation((p) => {
      const s = String(p);
      if (s === "/fake/.vmsan/bin/vmsan-nftables") return true;
      return false;
    });

    vi.mocked(execFileSync).mockImplementation(() => {
      throw new Error("binary crashed");
    });

    // Should not throw, should just skip the nftables check
    const leaks = verifyCleanup(makeNetwork(), "my-vm");
    expect(leaks).not.toContainEqual(expect.stringContaining("nftables"));
  });
});
