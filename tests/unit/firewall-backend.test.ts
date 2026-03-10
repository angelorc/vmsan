import { describe, it, expect } from "vitest";
import { buildInitialVmState } from "../../src/commands/create/state.ts";
import type { InitialVmStateInput } from "../../src/commands/create/types.ts";
import type { VmNetwork } from "../../src/lib/vm-state.ts";

function makeInput(overrides?: Partial<InitialVmStateInput>): InitialVmStateInput {
  return {
    vmId: "test-vm-001",
    project: "default",
    runtime: "base",
    diskSizeGb: 10,
    kernelPath: "/fake/vmlinux",
    rootfsPath: "/fake/rootfs.ext4",
    vcpus: 1,
    memMib: 128,
    networkPolicy: "allow-all",
    domains: [],
    allowedCidrs: [],
    deniedCidrs: [],
    ports: [],
    tapDevice: "fhvm0",
    hostIp: "198.19.0.1",
    guestIp: "198.19.0.2",
    subnetMask: "255.255.255.252",
    macAddress: "AA:FC:00:00:00:01",
    snapshotId: null,
    timeoutMs: null,
    agentToken: null,
    agentPort: 9119,
    ...overrides,
  };
}

describe("firewallBackend in buildInitialVmState", () => {
  it("sets firewallBackend to 'nftables' for new VMs", () => {
    const state = buildInitialVmState(makeInput());
    expect(state.network.firewallBackend).toBe("nftables");
  });

  it("includes firewallBackend in the network config object", () => {
    const state = buildInitialVmState(makeInput());
    expect(state.network).toHaveProperty("firewallBackend");
  });

  it("produces a valid VmState with all required fields", () => {
    const state = buildInitialVmState(makeInput({ vmId: "abc123" }));
    expect(state.id).toBe("abc123");
    expect(state.status).toBe("creating");
    expect(state.network.tapDevice).toBe("fhvm0");
    expect(state.network.firewallBackend).toBe("nftables");
    expect(state.stateVersion).toBe(2);
  });

  it("carries through all network fields alongside firewallBackend", () => {
    const input = makeInput({
      networkPolicy: "custom",
      domains: ["example.com"],
      allowedCidrs: ["10.0.0.0/8"],
      deniedCidrs: ["192.168.0.0/16"],
      ports: [80, 443],
    });
    const state = buildInitialVmState(input);
    expect(state.network.networkPolicy).toBe("custom");
    expect(state.network.allowedDomains).toEqual(["example.com"]);
    expect(state.network.allowedCidrs).toEqual(["10.0.0.0/8"]);
    expect(state.network.deniedCidrs).toEqual(["192.168.0.0/16"]);
    expect(state.network.publishedPorts).toEqual([80, 443]);
    expect(state.network.firewallBackend).toBe("nftables");
  });
});

describe("VmNetwork firewallBackend backward compatibility", () => {
  it("allows undefined firewallBackend for 0.1.0 state files", () => {
    // Simulate a 0.1.0 state file that has no firewallBackend field
    const legacyNetwork: VmNetwork = {
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
    };

    // firewallBackend is optional — undefined is valid
    expect(legacyNetwork.firewallBackend).toBeUndefined();
  });

  it("allows 'iptables' as firewallBackend value", () => {
    const network: VmNetwork = {
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
      firewallBackend: "iptables",
    };

    expect(network.firewallBackend).toBe("iptables");
  });

  it("allows 'nftables' as firewallBackend value", () => {
    const network: VmNetwork = {
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
      firewallBackend: "nftables",
    };

    expect(network.firewallBackend).toBe("nftables");
  });
});
