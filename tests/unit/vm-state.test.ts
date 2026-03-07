import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import {
  CURRENT_STATE_VERSION,
  FileVmStateStore,
  findFreeNetworkSlot,
  type VmState,
} from "../../src/lib/vm-state.ts";

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
        stateVersion: 1,
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
        stateVersion: 1,
      },
    ]);

    expect(slot).toBe(2);
  });
});

function makeLegacyState(): Record<string, unknown> {
  return {
    id: "vm-legacy",
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
  };
}

const tempDirs: string[] = [];

function makeTempStore(): FileVmStateStore {
  const dir = mkdtempSync(join(tmpdir(), "vmsan-state-test-"));
  tempDirs.push(dir);
  return new FileVmStateStore(dir);
}

afterEach(() => {
  while (tempDirs.length > 0) {
    const dir = tempDirs.pop();
    if (dir) rmSync(dir, { recursive: true, force: true });
  }
});

describe("state file versioning", () => {
  it("loads legacy state without version field", () => {
    const dir = mkdtempSync(join(tmpdir(), "vmsan-state-test-"));
    tempDirs.push(dir);
    const legacy = makeLegacyState();
    writeFileSync(join(dir, "vm-legacy.json"), JSON.stringify(legacy));

    const store = new FileVmStateStore(dir);
    const state = store.load("vm-legacy");

    expect(state).not.toBeNull();
    expect(state!.id).toBe("vm-legacy");
    expect(state!.stateVersion).toBe(CURRENT_STATE_VERSION);
  });

  it("saves state with version 1", () => {
    const store = makeTempStore();
    const state = { ...makeLegacyState(), stateVersion: CURRENT_STATE_VERSION } as VmState;
    store.save(state);

    const dir = tempDirs[tempDirs.length - 1];
    const raw = JSON.parse(readFileSync(join(dir, "vm-legacy.json"), "utf-8"));
    expect(raw.stateVersion).toBe(1);
  });

  it("auto-migrates v0 to v1", () => {
    const dir = mkdtempSync(join(tmpdir(), "vmsan-state-test-"));
    tempDirs.push(dir);
    const legacy = makeLegacyState();
    writeFileSync(join(dir, "vm-legacy.json"), JSON.stringify(legacy));

    const store = new FileVmStateStore(dir);
    store.load("vm-legacy");

    // Verify the file on disk was re-written with stateVersion
    const raw = JSON.parse(readFileSync(join(dir, "vm-legacy.json"), "utf-8"));
    expect(raw.stateVersion).toBe(1);
  });
});
