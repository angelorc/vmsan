import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { randomBytes } from "node:crypto";

// We cannot easily redirect SecretsStore (hardcoded homedir). Instead, test
// the encryption round-trip by monkey-patching the internal dirs via Object
// property assignment after construction. SecretsStore stores keysDir and
// secretsDir as private fields — we access them via (store as any).

import { SecretsStore } from "../../src/lib/secrets/store.ts";

function makeTempStore(): { store: SecretsStore; tmpDir: string } {
  const tmpDir = join(tmpdir(), `vmsan-test-secrets-${randomBytes(4).toString("hex")}`);
  mkdirSync(tmpDir, { recursive: true });

  const store = new SecretsStore();
  // Redirect internal paths to temp directory
  (store as any).keysDir = join(tmpDir, "keys");
  (store as any).secretsDir = join(tmpDir, "secrets");

  return { store, tmpDir };
}

describe("SecretsStore", () => {
  let store: SecretsStore;
  let tmpDir: string;

  beforeEach(() => {
    const ctx = makeTempStore();
    store = ctx.store;
    tmpDir = ctx.tmpDir;
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  it("set + get round-trip returns the original value", () => {
    store.set("myproject", "API_KEY", "secret-value-123");
    const result = store.get("myproject", "API_KEY");
    expect(result).toBe("secret-value-123");
  });

  it("get returns null for a non-existent key", () => {
    const result = store.get("myproject", "MISSING_KEY");
    expect(result).toBeNull();
  });

  it("list returns sorted key names", () => {
    store.set("myproject", "ZEBRA", "z");
    store.set("myproject", "ALPHA", "a");
    store.set("myproject", "MIDDLE", "m");
    const keys = store.list("myproject");
    expect(keys).toEqual(["ALPHA", "MIDDLE", "ZEBRA"]);
  });

  it("list returns empty array for unknown project", () => {
    const keys = store.list("nonexistent");
    expect(keys).toEqual([]);
  });

  it("unset removes a key and returns true", () => {
    store.set("myproject", "TO_REMOVE", "value");
    const removed = store.unset("myproject", "TO_REMOVE");
    expect(removed).toBe(true);
    expect(store.get("myproject", "TO_REMOVE")).toBeNull();
    expect(store.list("myproject")).toEqual([]);
  });

  it("unset returns false for non-existent key", () => {
    const removed = store.unset("myproject", "NOPE");
    expect(removed).toBe(false);
  });

  it("getAll returns all secrets as key-value map", () => {
    store.set("myproject", "KEY_A", "value-a");
    store.set("myproject", "KEY_B", "value-b");
    const all = store.getAll("myproject");
    expect(all).toEqual({
      KEY_A: "value-a",
      KEY_B: "value-b",
    });
  });

  it("getAll returns empty object for unknown project", () => {
    const all = store.getAll("unknown");
    expect(all).toEqual({});
  });

  it("different projects have separate stores", () => {
    store.set("project-a", "TOKEN", "aaa");
    store.set("project-b", "TOKEN", "bbb");

    expect(store.get("project-a", "TOKEN")).toBe("aaa");
    expect(store.get("project-b", "TOKEN")).toBe("bbb");

    expect(store.list("project-a")).toEqual(["TOKEN"]);
    expect(store.list("project-b")).toEqual(["TOKEN"]);
  });

  it("handles special characters in values", () => {
    const special = `p@$$w0rd!#%^&*(){}[]|\\:";'<>?,./~\`\n\ttabs`;
    store.set("myproject", "SPECIAL", special);
    expect(store.get("myproject", "SPECIAL")).toBe(special);
  });

  it("handles empty string value", () => {
    store.set("myproject", "EMPTY", "");
    expect(store.get("myproject", "EMPTY")).toBe("");
  });

  it("overwrites existing key with new value", () => {
    store.set("myproject", "KEY", "old-value");
    store.set("myproject", "KEY", "new-value");
    expect(store.get("myproject", "KEY")).toBe("new-value");
    expect(store.list("myproject")).toEqual(["KEY"]);
  });
});
