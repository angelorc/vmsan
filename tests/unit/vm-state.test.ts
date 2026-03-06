import { describe, expect, it } from "vitest";
import { findFreeNetworkSlot } from "../../src/lib/vm-state.ts";

describe("findFreeNetworkSlot", () => {
  it("keeps stopped vm slots reserved for restart", () => {
    const slot = findFreeNetworkSlot([
      {
        id: "vm-stopped",
        project: "",
        runtime: "base",
        status: "stopped",
        pid: null,
        apiSocket: "",
        chrootDir: "",
        kernel: "",
        rootfs: "",
        vcpuCount: 1,
        memSizeMib: 128,
        network: {
          tapDevice: "fhvm0",
          hostIp: "198.19.0.1",
          guestIp: "198.19.0.2",
          subnetMask: "255.255.255.252",
          macAddress: "",
          networkPolicy: "allow-all",
          allowedDomains: [],
          allowedCidrs: [],
          deniedCidrs: [],
          publishedPorts: [],
          tunnelHostname: null,
        },
        snapshot: null,
        timeoutMs: null,
        timeoutAt: null,
        createdAt: "",
        error: null,
        agentToken: null,
        agentPort: 9119,
      },
      {
        id: "vm-running",
        project: "",
        runtime: "base",
        status: "running",
        pid: 1,
        apiSocket: "",
        chrootDir: "",
        kernel: "",
        rootfs: "",
        vcpuCount: 1,
        memSizeMib: 128,
        network: {
          tapDevice: "fhvm1",
          hostIp: "198.19.1.1",
          guestIp: "198.19.1.2",
          subnetMask: "255.255.255.252",
          macAddress: "",
          networkPolicy: "allow-all",
          allowedDomains: [],
          allowedCidrs: [],
          deniedCidrs: [],
          publishedPorts: [],
          tunnelHostname: null,
        },
        snapshot: null,
        timeoutMs: null,
        timeoutAt: null,
        createdAt: "",
        error: null,
        agentToken: null,
        agentPort: 9119,
      },
    ]);

    expect(slot).toBe(2);
  });
});
