import { describe, test, expect, vi, beforeEach } from "vitest";
import { existsSync } from "node:fs";
import type { VmNetwork } from "../../src/lib/vm-state.ts";
import { verifyCleanup } from "../../src/lib/network.ts";

vi.mock("node:fs", async (importOriginal) => {
  const actual = await importOriginal<typeof import("node:fs")>();
  return {
    ...actual,
    existsSync: vi.fn(),
  };
});

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
});
