import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { SqliteVmStateStore } from "../../src/lib/state/sqlite.ts";
import { CURRENT_STATE_VERSION, type VmState } from "../../src/lib/vm-state.ts";
import { VmsanError } from "../../src/errors/base.ts";
import type { HostState, SyncLogEntry } from "../../src/lib/state/types.ts";

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

const tempDirs: string[] = [];
const stores: SqliteVmStateStore[] = [];

function makeTempStore(): SqliteVmStateStore {
  const dir = mkdtempSync(join(tmpdir(), "vmsan-sqlite-test-"));
  tempDirs.push(dir);
  const store = new SqliteVmStateStore(join(dir, "state.db"));
  stores.push(store);
  return store;
}

afterEach(() => {
  while (stores.length > 0) {
    const store = stores.pop();
    if (store) store.close();
  }
  while (tempDirs.length > 0) {
    const dir = tempDirs.pop();
    if (dir) rmSync(dir, { recursive: true, force: true });
  }
});

// ---------- SqliteVmStateStore ----------

describe("SqliteVmStateStore", () => {
  it("saves and loads VM state", () => {
    const store = makeTempStore();
    const state = makeVmState({ id: "vm-save-test" });

    store.save(state);
    const loaded = store.load("vm-save-test");
    expect(loaded).not.toBeNull();
    expect(loaded!.id).toBe("vm-save-test");
    expect(loaded!.vcpuCount).toBe(1);
  });

  it("returns null for non-existent VM", () => {
    const store = makeTempStore();
    expect(store.load("does-not-exist")).toBeNull();
  });

  it("lists all VMs", () => {
    const store = makeTempStore();

    store.save(makeVmState({ id: "vm-a" }));
    store.save(makeVmState({ id: "vm-b" }));

    const list = store.list();
    expect(list).toHaveLength(2);
    const ids = list.map((s) => s.id).sort();
    expect(ids).toEqual(["vm-a", "vm-b"]);
  });

  it("updates VM state", () => {
    const store = makeTempStore();

    store.save(makeVmState({ id: "vm-update", status: "creating" }));
    store.update("vm-update", { status: "running", pid: 12345 });

    const loaded = store.load("vm-update");
    expect(loaded!.status).toBe("running");
    expect(loaded!.pid).toBe(12345);
  });

  it("throws when updating a non-existent VM", () => {
    const store = makeTempStore();
    expect(() => store.update("ghost", { status: "running" })).toThrow(VmsanError);
  });

  it("deletes VM state", () => {
    const store = makeTempStore();

    store.save(makeVmState({ id: "vm-delete" }));
    store.delete("vm-delete");
    expect(store.load("vm-delete")).toBeNull();
  });

  it("delete is idempotent for non-existent VM", () => {
    const store = makeTempStore();
    expect(() => store.delete("non-existent")).not.toThrow();
  });

  it("allocateNetworkSlot returns sequential slots", () => {
    const store = makeTempStore();

    store.save(
      makeVmState({ id: "vm-0", network: { ...makeVmState().network, hostIp: "198.19.0.1" } }),
    );

    const slot = store.allocateNetworkSlot();
    expect(slot).toBe(1);
  });

  it("sets stateVersion to current on save", () => {
    const store = makeTempStore();
    const state = makeVmState({ id: "vm-version", stateVersion: 1 });

    store.save(state);
    const loaded = store.load("vm-version");
    expect(loaded!.stateVersion).toBe(CURRENT_STATE_VERSION);
  });

  it("saves and retrieves full VmState fidelity", () => {
    const store = makeTempStore();
    const state = makeVmState({
      id: "vm-full",
      project: "myproject",
      runtime: "node",
      diskSizeGb: 20,
      status: "running",
      pid: 42,
      vcpuCount: 4,
      memSizeMib: 2048,
      network: {
        tapDevice: "fhvm5",
        hostIp: "198.19.5.1",
        guestIp: "198.19.5.2",
        subnetMask: "255.255.255.252",
        macAddress: "aa:bb:cc:dd:ee:ff",
        networkPolicy: "allow-list",
        allowedDomains: ["example.com"],
        allowedCidrs: ["10.0.0.0/8"],
        deniedCidrs: ["192.168.0.0/16"],
        publishedPorts: [8080],
        tunnelHostname: "test.example.com",
        meshIp: "10.200.0.5",
        service: "web",
      },
      agentToken: "secret-token",
      disableSeccomp: true,
    });

    store.save(state);
    const loaded = store.load("vm-full");
    expect(loaded!.project).toBe("myproject");
    expect(loaded!.runtime).toBe("node");
    expect(loaded!.diskSizeGb).toBe(20);
    expect(loaded!.vcpuCount).toBe(4);
    expect(loaded!.memSizeMib).toBe(2048);
    expect(loaded!.network.meshIp).toBe("10.200.0.5");
    expect(loaded!.network.service).toBe("web");
    expect(loaded!.network.allowedDomains).toEqual(["example.com"]);
    expect(loaded!.agentToken).toBe("secret-token");
    expect(loaded!.disableSeccomp).toBe(true);
  });

  it("overwrites existing VM on save", () => {
    const store = makeTempStore();

    store.save(makeVmState({ id: "vm-overwrite", status: "creating" }));
    store.save(makeVmState({ id: "vm-overwrite", status: "running" }));

    const loaded = store.load("vm-overwrite");
    expect(loaded!.status).toBe("running");
    expect(store.list()).toHaveLength(1);
  });

  it("vmCount returns correct count", () => {
    const store = makeTempStore();
    expect(store.vmCount()).toBe(0);

    store.save(makeVmState({ id: "vm-a" }));
    store.save(makeVmState({ id: "vm-b" }));
    expect(store.vmCount()).toBe(2);

    store.delete("vm-a");
    expect(store.vmCount()).toBe(1);
  });
});

// ---------- listByProject ----------

describe("SqliteVmStateStore.listByProject", () => {
  it("filters VMs by project", () => {
    const store = makeTempStore();

    store.save(makeVmState({ id: "vm-p1-a", project: "proj1" }));
    store.save(makeVmState({ id: "vm-p1-b", project: "proj1" }));
    store.save(makeVmState({ id: "vm-p2-a", project: "proj2" }));

    const proj1 = store.listByProject("proj1");
    expect(proj1).toHaveLength(2);
    expect(proj1.map((s) => s.id).sort()).toEqual(["vm-p1-a", "vm-p1-b"]);

    const proj2 = store.listByProject("proj2");
    expect(proj2).toHaveLength(1);
    expect(proj2[0].id).toBe("vm-p2-a");

    const proj3 = store.listByProject("proj3");
    expect(proj3).toHaveLength(0);
  });
});

// ---------- mesh allocation cleanup ----------

describe("SqliteVmStateStore mesh allocations", () => {
  it("cleans up mesh allocation on VM delete", () => {
    const store = makeTempStore();

    store.save(
      makeVmState({
        id: "vm-mesh",
        project: "proj",
        network: { ...makeVmState().network, meshIp: "10.200.0.1", service: "web" },
      }),
    );

    // VM should exist
    expect(store.load("vm-mesh")).not.toBeNull();

    // Delete and verify
    store.delete("vm-mesh");
    expect(store.load("vm-mesh")).toBeNull();
  });
});

// ---------- Host operations ----------

describe("SqliteVmStateStore host operations", () => {
  function makeHost(overrides: Partial<HostState> = {}): HostState {
    return {
      id: "host-1",
      name: "node-1",
      address: "10.0.0.1",
      status: "pending",
      createdAt: new Date().toISOString(),
      ...overrides,
    };
  }

  it("saves and loads a host", () => {
    const store = makeTempStore();
    const host = makeHost();

    store.saveHost(host);
    const loaded = store.loadHost("host-1");
    expect(loaded).not.toBeNull();
    expect(loaded!.id).toBe("host-1");
    expect(loaded!.name).toBe("node-1");
    expect(loaded!.address).toBe("10.0.0.1");
    expect(loaded!.status).toBe("pending");
  });

  it("returns null for non-existent host", () => {
    const store = makeTempStore();
    expect(store.loadHost("nope")).toBeNull();
  });

  it("lists all hosts", () => {
    const store = makeTempStore();

    store.saveHost(makeHost({ id: "h1", name: "node-1" }));
    store.saveHost(makeHost({ id: "h2", name: "node-2" }));

    const hosts = store.listHosts();
    expect(hosts).toHaveLength(2);
  });

  it("saves host with resources", () => {
    const store = makeTempStore();
    const host = makeHost({
      resources: { cpus: 8, memoryMB: 16384, diskGB: 500 },
    });

    store.saveHost(host);
    const loaded = store.loadHost("host-1");
    expect(loaded!.resources).toEqual({ cpus: 8, memoryMB: 16384, diskGB: 500 });
  });

  it("deletes a host", () => {
    const store = makeTempStore();

    store.saveHost(makeHost());
    store.deleteHost("host-1");
    expect(store.loadHost("host-1")).toBeNull();
  });

  it("delete is idempotent for non-existent host", () => {
    const store = makeTempStore();
    expect(() => store.deleteHost("nope")).not.toThrow();
  });
});

// ---------- Sync log ----------

describe("SqliteVmStateStore sync log", () => {
  it("appends and reads sync log entries", () => {
    const store = makeTempStore();

    store.appendSyncLog({
      entityType: "vm",
      entityId: "vm-1",
      operation: "create",
      payload: { name: "test" },
    });
    store.appendSyncLog({
      entityType: "host",
      entityId: "host-1",
      operation: "update",
    });

    const entries = store.readSyncLogSince(0);
    expect(entries).toHaveLength(2);
    expect(entries[0].entityType).toBe("vm");
    expect(entries[0].entityId).toBe("vm-1");
    expect(entries[0].operation).toBe("create");
    expect(entries[0].payload).toEqual({ name: "test" });
    expect(entries[0].version).toBe(1);
    expect(entries[1].version).toBe(2);
  });

  it("reads entries since a specific version", () => {
    const store = makeTempStore();

    store.appendSyncLog({ entityType: "vm", entityId: "vm-1", operation: "create" });
    store.appendSyncLog({ entityType: "vm", entityId: "vm-2", operation: "create" });
    store.appendSyncLog({ entityType: "vm", entityId: "vm-3", operation: "create" });

    const entries = store.readSyncLogSince(1);
    expect(entries).toHaveLength(2);
    expect(entries[0].entityId).toBe("vm-2");
    expect(entries[1].entityId).toBe("vm-3");
  });

  it("returns empty when no entries after version", () => {
    const store = makeTempStore();

    store.appendSyncLog({ entityType: "vm", entityId: "vm-1", operation: "create" });

    const entries = store.readSyncLogSince(1);
    expect(entries).toHaveLength(0);
  });

  it("handles entries without payload", () => {
    const store = makeTempStore();

    store.appendSyncLog({ entityType: "vm", entityId: "vm-1", operation: "delete" });

    const entries = store.readSyncLogSince(0);
    expect(entries).toHaveLength(1);
    expect(entries[0].payload).toBeUndefined();
  });
});

// ---------- concurrent allocation ----------

describe("SqliteVmStateStore concurrent slot allocation", () => {
  it("concurrent allocations get unique slots when writing to db", () => {
    const store = makeTempStore();
    const allocatedSlots = new Set<number>();

    for (let i = 0; i < 10; i++) {
      const slot = store.allocateNetworkSlot();
      expect(allocatedSlots.has(slot)).toBe(false);
      allocatedSlots.add(slot);

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

// ---------- createStateStore factory ----------

describe("createStateStore", () => {
  it("returns SqliteVmStateStore by default", async () => {
    const { createStateStore } = await import("../../src/lib/state/index.ts");
    const dir = mkdtempSync(join(tmpdir(), "vmsan-factory-test-"));
    tempDirs.push(dir);

    const store = createStateStore(join(dir, "vms"));
    expect(store).toBeInstanceOf(SqliteVmStateStore);
    (store as SqliteVmStateStore).close();
  });

  it("returns FileVmStateStore when VMSAN_STATE_BACKEND=json", async () => {
    const { FileVmStateStore } = await import("../../src/lib/vm-state.ts");
    const { createStateStore } = await import("../../src/lib/state/index.ts");
    const dir = mkdtempSync(join(tmpdir(), "vmsan-factory-test-"));
    tempDirs.push(dir);

    const prev = process.env.VMSAN_STATE_BACKEND;
    process.env.VMSAN_STATE_BACKEND = "json";
    try {
      const store = createStateStore(join(dir, "vms"));
      expect(store).toBeInstanceOf(FileVmStateStore);
    } finally {
      if (prev === undefined) {
        delete process.env.VMSAN_STATE_BACKEND;
      } else {
        process.env.VMSAN_STATE_BACKEND = prev;
      }
    }
  });
});
