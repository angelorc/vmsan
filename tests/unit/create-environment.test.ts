import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { findBaseRootfs } from "../../src/commands/create/environment.ts";

const tempDirs: string[] = [];

function makeBaseDir(files: string[]): string {
  const baseDir = mkdtempSync(join(tmpdir(), "vmsan-rootfs-test-"));
  tempDirs.push(baseDir);
  const rootfsDir = join(baseDir, "rootfs");
  mkdirSync(rootfsDir, { recursive: true });
  for (const file of files) {
    writeFileSync(join(rootfsDir, file), "");
  }
  return baseDir;
}

afterEach(() => {
  while (tempDirs.length > 0) {
    const baseDir = tempDirs.pop();
    if (baseDir) {
      rmSync(baseDir, { recursive: true, force: true });
    }
  }
});

describe("findBaseRootfs", () => {
  it("prefers the canonical base rootfs when runtime images are present", () => {
    const baseDir = makeBaseDir(["node22.ext4", "node24.ext4", "ubuntu-24.04.ext4"]);
    expect(findBaseRootfs(baseDir)).toBe(join(baseDir, "rootfs", "ubuntu-24.04.ext4"));
  });

  it("falls back to non-runtime ext4 images only", () => {
    const baseDir = makeBaseDir(["node22.ext4", "custom-base.ext4"]);
    expect(findBaseRootfs(baseDir)).toBe(join(baseDir, "rootfs", "custom-base.ext4"));
  });
});
