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
import { VmsanError } from "../../src/errors/base.ts";
import { buildInitialVmState } from "../../src/commands/create/state.ts";
import type { InitialVmStateInput } from "../../src/commands/create/types.ts";

// ---------- helpers ----------

function makeVmState(overrides: Partial<VmState> = {}): VmState {
  return {
    id: overrides.id ?? "vm-test",
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
    createdAt: new Date().toISOString(),
    error: null,
    agentToken: null,
    agentPort: 9119,
    stateVersion: CURRENT_STATE_VERSION,
    disableSeccomp: false,
    disablePidNs: false,
    disableCgroup: false,
    ...overrides,
  };
}

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

function makeTempDir(): string {
  const dir = mkdtempSync(join(tmpdir(), "vmsan-state-test-"));
  tempDirs.push(dir);
  return dir;
}

afterEach(() => {
  while (tempDirs.length > 0) {
    const dir = tempDirs.pop();
    if (dir) rmSync(dir, { recursive: true, force: true });
  }
});

// ---------- findFreeNetworkSlot ----------

describe("findFreeNetworkSlot", () => {
  it("returns slot 0 when no VMs exist", () => {
    const slot = findFreeNetworkSlot([]);
    expect(slot).toBe(0);
  });

  it("keeps stopped vm slots reserved for restart", () => {
    const slot = findFreeNetworkSlot([
      makeVmState({
        id: "vm-stopped",
        status: "stopped",
        network: { ...makeVmState().network, hostIp: "198.19.0.1" },
      }),
      makeVmState({
        id: "vm-running",
        status: "running",
        network: { ...makeVmState().network, hostIp: "198.19.1.1" },
      }),
    ]);
    expect(slot).toBe(2);
  });

  it("skips error-status VMs and reuses their slots", () => {
    const slot = findFreeNetworkSlot([
      makeVmState({
        id: "vm-error",
        status: "error",
        network: { ...makeVmState().network, hostIp: "198.19.0.1" },
      }),
    ]);
    expect(slot).toBe(0);
  });

  it("finds gaps between allocated slots", () => {
    const states = [
      makeVmState({ id: "vm-0", network: { ...makeVmState().network, hostIp: "198.19.0.1" } }),
      makeVmState({ id: "vm-2", network: { ...makeVmState().network, hostIp: "198.19.2.1" } }),
    ];
    const slot = findFreeNetworkSlot(states);
    expect(slot).toBe(1);
  });

  it("throws when all 255 slots are taken", () => {
    const states: VmState[] = [];
    for (let i = 0; i <= 254; i++) {
      states.push(
        makeVmState({
          id: `vm-${i}`,
          network: { ...makeVmState().network, hostIp: `198.19.${i}.1` },
        }),
      );
    }
    expect(() => findFreeNetworkSlot(states)).toThrow(VmsanError);
  });
});

// ---------- FileVmStateStore ----------

describe("FileVmStateStore", () => {
  it("saves and loads VM state", () => {
    const dir = makeTempDir();
    const store = new FileVmStateStore(dir);
    const state = makeVmState({ id: "vm-save-test" });

    store.save(state);
    const loaded = store.load("vm-save-test");
    expect(loaded).not.toBeNull();
    expect(loaded!.id).toBe("vm-save-test");
    expect(loaded!.vcpuCount).toBe(1);
  });

  it("returns null for non-existent VM", () => {
    const dir = makeTempDir();
    const store = new FileVmStateStore(dir);
    expect(store.load("does-not-exist")).toBeNull();
  });

  it("lists all VMs", () => {
    const dir = makeTempDir();
    const store = new FileVmStateStore(dir);

    store.save(makeVmState({ id: "vm-a" }));
    store.save(makeVmState({ id: "vm-b" }));

    const list = store.list();
    expect(list).toHaveLength(2);
    const ids = list.map((s) => s.id).sort();
    expect(ids).toEqual(["vm-a", "vm-b"]);
  });

  it("updates VM state", () => {
    const dir = makeTempDir();
    const store = new FileVmStateStore(dir);

    store.save(makeVmState({ id: "vm-update", status: "creating" }));
    store.update("vm-update", { status: "running", pid: 12345 });

    const loaded = store.load("vm-update");
    expect(loaded!.status).toBe("running");
    expect(loaded!.pid).toBe(12345);
  });

  it("throws when updating a non-existent VM", () => {
    const dir = makeTempDir();
    const store = new FileVmStateStore(dir);

    expect(() => store.update("ghost", { status: "running" })).toThrow(VmsanError);
  });

  it("deletes VM state", () => {
    const dir = makeTempDir();
    const store = new FileVmStateStore(dir);

    store.save(makeVmState({ id: "vm-delete" }));
    store.delete("vm-delete");
    expect(store.load("vm-delete")).toBeNull();
  });

  it("delete is idempotent for non-existent VM", () => {
    const dir = makeTempDir();
    const store = new FileVmStateStore(dir);
    expect(() => store.delete("non-existent")).not.toThrow();
  });

  it("allocateNetworkSlot returns sequential slots", () => {
    const dir = makeTempDir();
    const store = new FileVmStateStore(dir);

    // Save a VM in slot 0
    store.save(
      makeVmState({ id: "vm-0", network: { ...makeVmState().network, hostIp: "198.19.0.1" } }),
    );

    const slot = store.allocateNetworkSlot();
    expect(slot).toBe(1);
  });
});

// ---------- concurrent allocation ----------

describe("concurrent slot allocation", () => {
  it("concurrent allocations get unique slots when writing to disk", () => {
    const dir = makeTempDir();
    const store = new FileVmStateStore(dir);
    const allocatedSlots = new Set<number>();

    // Simulate sequential allocation (true concurrency requires locks; here we verify
    // that each allocation after saving the previous VM returns a unique slot)
    for (let i = 0; i < 10; i++) {
      const slot = store.allocateNetworkSlot();
      expect(allocatedSlots.has(slot)).toBe(false);
      allocatedSlots.add(slot);

      // Save a VM at this slot so the next allocation knows it is taken
      store.save(
        makeVmState({
          id: `vm-${i}`,
          network: { ...makeVmState().network, hostIp: `198.19.${slot}.1` },
        }),
      );
    }

    expect(allocatedSlots.size).toBe(10);
  });
});

// ---------- state file versioning ----------

describe("state file versioning", () => {
  it("loads legacy state without version field", () => {
    const dir = makeTempDir();
    const legacy = makeLegacyState();
    writeFileSync(join(dir, "vm-legacy.json"), JSON.stringify(legacy));

    const store = new FileVmStateStore(dir);
    const state = store.load("vm-legacy");

    expect(state).not.toBeNull();
    expect(state!.id).toBe("vm-legacy");
    expect(state!.stateVersion).toBe(CURRENT_STATE_VERSION);
  });

  it("saves state with current version", () => {
    const dir = makeTempDir();
    const store = new FileVmStateStore(dir);
    const state = { ...makeLegacyState(), stateVersion: CURRENT_STATE_VERSION } as VmState;
    store.save(state);

    const raw = JSON.parse(readFileSync(join(dir, "vm-legacy.json"), "utf-8"));
    expect(raw.stateVersion).toBe(CURRENT_STATE_VERSION);
  });

  it("auto-migrates v0 through all versions", () => {
    const dir = makeTempDir();
    const legacy = makeLegacyState();
    writeFileSync(join(dir, "vm-legacy.json"), JSON.stringify(legacy));

    const store = new FileVmStateStore(dir);
    store.load("vm-legacy");

    // Verify the file on disk was re-written with current version
    const raw = JSON.parse(readFileSync(join(dir, "vm-legacy.json"), "utf-8"));
    expect(raw.stateVersion).toBe(CURRENT_STATE_VERSION);
  });

  it("migrates v1 to v2 adding isolation flags defaulting to false", () => {
    const dir = makeTempDir();
    const v1State = { ...makeLegacyState(), id: "vm-v1", stateVersion: 1 };
    writeFileSync(join(dir, "vm-v1.json"), JSON.stringify(v1State));

    const store = new FileVmStateStore(dir);
    const state = store.load("vm-v1");

    expect(state).not.toBeNull();
    expect(state!.stateVersion).toBe(2);
    expect(state!.disableSeccomp).toBe(false);
    expect(state!.disablePidNs).toBe(false);
    expect(state!.disableCgroup).toBe(false);

    // Verify persisted on disk
    const raw = JSON.parse(readFileSync(join(dir, "vm-v1.json"), "utf-8"));
    expect(raw.stateVersion).toBe(2);
    expect(raw.disableSeccomp).toBe(false);
    expect(raw.disablePidNs).toBe(false);
    expect(raw.disableCgroup).toBe(false);
  });

  it("preserves existing isolation flags during v1 to v2 migration", () => {
    const dir = makeTempDir();
    // Simulate a state that somehow already has disableSeccomp set (e.g. manual edit)
    const v1State = { ...makeLegacyState(), id: "vm-v1-custom", stateVersion: 1, disableSeccomp: true };
    writeFileSync(join(dir, "vm-v1-custom.json"), JSON.stringify(v1State));

    const store = new FileVmStateStore(dir);
    const state = store.load("vm-v1-custom");

    expect(state!.disableSeccomp).toBe(true);
    expect(state!.disablePidNs).toBe(false);
    expect(state!.disableCgroup).toBe(false);
  });
});

// ---------- buildInitialVmState isolation flags ----------

describe("buildInitialVmState", () => {
  function makeInput(overrides: Partial<InitialVmStateInput> = {}): InitialVmStateInput {
    return {
      vmId: "vm-test",
      project: "",
      runtime: "base",
      diskSizeGb: 10,
      kernelPath: "/kernel",
      rootfsPath: "/rootfs",
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
      macAddress: "aa:bb:cc:dd:ee:ff",
      snapshotId: null,
      timeoutMs: null,
      agentToken: null,
      agentPort: 9119,
      ...overrides,
    };
  }

  it("defaults isolation flags to false", () => {
    const state = buildInitialVmState(makeInput());
    expect(state.disableSeccomp).toBe(false);
    expect(state.disablePidNs).toBe(false);
    expect(state.disableCgroup).toBe(false);
  });

  it("persists disableSeccomp when set to true", () => {
    const state = buildInitialVmState(makeInput({ disableSeccomp: true }));
    expect(state.disableSeccomp).toBe(true);
    expect(state.disablePidNs).toBe(false);
    expect(state.disableCgroup).toBe(false);
  });

  it("persists disablePidNs when set to true", () => {
    const state = buildInitialVmState(makeInput({ disablePidNs: true }));
    expect(state.disablePidNs).toBe(true);
  });

  it("persists disableCgroup when set to true", () => {
    const state = buildInitialVmState(makeInput({ disableCgroup: true }));
    expect(state.disableCgroup).toBe(true);
  });

  it("persists all isolation flags when all set to true", () => {
    const state = buildInitialVmState(
      makeInput({ disableSeccomp: true, disablePidNs: true, disableCgroup: true }),
    );
    expect(state.disableSeccomp).toBe(true);
    expect(state.disablePidNs).toBe(true);
    expect(state.disableCgroup).toBe(true);
  });
});
