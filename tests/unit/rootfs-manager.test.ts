import { describe, it, expect, beforeEach, afterAll } from "vitest";
import { createHash } from "node:crypto";
import { writeFileSync, rmSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { tmpdir, homedir } from "node:os";
import { randomBytes } from "node:crypto";
import { getCacheDir, verifyChecksum } from "../../src/lib/rootfs-manager.ts";

// ---------- getCacheDir ----------

describe("getCacheDir", () => {
  it("returns path under ~/.vmsan/rootfs", () => {
    const dir = getCacheDir();
    expect(dir).toBe(join(homedir(), ".vmsan", "rootfs"));
  });

  it("returns an absolute path", () => {
    const dir = getCacheDir();
    expect(dir.startsWith("/")).toBe(true);
  });
});

// ---------- verifyChecksum ----------

describe("verifyChecksum", () => {
  const tmpDir = join(tmpdir(), `vmsan-test-rootfs-${randomBytes(4).toString("hex")}`);

  beforeEach(() => {
    mkdirSync(tmpDir, { recursive: true });
  });

  afterAll(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  it("returns true for matching sha256 hash", async () => {
    const filePath = join(tmpDir, "testfile.bin");
    const content = "hello world rootfs test content";
    writeFileSync(filePath, content);

    const expected = createHash("sha256").update(content).digest("hex");
    const result = await verifyChecksum(filePath, expected);
    expect(result).toBe(true);
  });

  it("returns false for non-matching hash", async () => {
    const filePath = join(tmpDir, "testfile2.bin");
    writeFileSync(filePath, "some content");

    const wrongHash = "0000000000000000000000000000000000000000000000000000000000000000";
    const result = await verifyChecksum(filePath, wrongHash);
    expect(result).toBe(false);
  });

  it("returns false for non-existent file", async () => {
    const result = await verifyChecksum(join(tmpDir, "does-not-exist.bin"), "abc123");
    expect(result).toBe(false);
  });

  it("returns true for empty file with correct hash", async () => {
    const filePath = join(tmpDir, "empty.bin");
    writeFileSync(filePath, "");

    const expected = createHash("sha256").update("").digest("hex");
    const result = await verifyChecksum(filePath, expected);
    expect(result).toBe(true);
  });
});
