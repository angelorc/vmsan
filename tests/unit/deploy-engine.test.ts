import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { mkdirSync, rmSync, existsSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { randomBytes } from "node:crypto";

// The deploy hash module uses a hardcoded HASH_FILE path based on homedir().
// We mock os.homedir() to redirect writes to a temp directory.

const tmpDir = join(tmpdir(), `vmsan-test-deploy-${randomBytes(4).toString("hex")}`);

vi.mock("node:os", async () => {
  const actual = await vi.importActual<typeof import("node:os")>("node:os");
  return {
    ...actual,
    homedir: () => tmpDir,
  };
});

// Import after the mock is in place so the module-level HASH_FILE constant
// picks up the mocked homedir.
const { getDeployHash, setDeployHash, removeDeployHash } = await import(
  "../../src/lib/deploy/hash.ts"
);

describe("deploy hash", () => {
  beforeEach(() => {
    mkdirSync(join(tmpDir, ".vmsan"), { recursive: true });
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  it("setDeployHash + getDeployHash round-trip", () => {
    setDeployHash("vm-abc12345", "sha256-deadbeef");
    const result = getDeployHash("vm-abc12345");
    expect(result).toBe("sha256-deadbeef");
  });

  it("getDeployHash returns null for unknown vmId", () => {
    const result = getDeployHash("vm-nonexistent");
    expect(result).toBeNull();
  });

  it("removeDeployHash removes the hash", () => {
    setDeployHash("vm-remove-me", "hash123");
    expect(getDeployHash("vm-remove-me")).toBe("hash123");

    removeDeployHash("vm-remove-me");
    expect(getDeployHash("vm-remove-me")).toBeNull();
  });

  it("handles multiple VMs independently", () => {
    setDeployHash("vm-aaa", "hash-a");
    setDeployHash("vm-bbb", "hash-b");

    expect(getDeployHash("vm-aaa")).toBe("hash-a");
    expect(getDeployHash("vm-bbb")).toBe("hash-b");
  });

  it("overwrites existing hash for the same vmId", () => {
    setDeployHash("vm-update", "old-hash");
    setDeployHash("vm-update", "new-hash");
    expect(getDeployHash("vm-update")).toBe("new-hash");
  });

  it("removeDeployHash is safe for non-existent vmId", () => {
    // Should not throw
    removeDeployHash("vm-nope");
    expect(getDeployHash("vm-nope")).toBeNull();
  });

  it("creates the hash file if it does not exist", () => {
    const hashFile = join(tmpDir, ".vmsan", "deploy-hashes.json");
    // Ensure file doesn't exist initially
    if (existsSync(hashFile)) {
      rmSync(hashFile);
    }
    setDeployHash("vm-new", "hash-new");
    expect(existsSync(hashFile)).toBe(true);
    expect(getDeployHash("vm-new")).toBe("hash-new");
  });
});
