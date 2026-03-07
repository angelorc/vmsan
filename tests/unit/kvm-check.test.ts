import { describe, test, expect, vi, beforeEach } from "vitest";
import { accessSync, existsSync } from "node:fs";
import { validateEnvironment } from "../../src/commands/create/environment.ts";

vi.mock("node:fs", async (importOriginal) => {
  const actual = await importOriginal<typeof import("node:fs")>();
  return {
    ...actual,
    existsSync: vi.fn(),
    accessSync: vi.fn(),
  };
});

describe("validateEnvironment KVM check", () => {
  beforeEach(() => {
    vi.mocked(existsSync).mockReset();
    vi.mocked(accessSync).mockReset();
  });

  test("throws ERR_SETUP_KVM_UNAVAILABLE on missing KVM", () => {
    // Firecracker and Jailer binaries exist
    vi.mocked(existsSync).mockReturnValue(true);

    // /dev/kvm is not accessible
    vi.mocked(accessSync).mockImplementation(() => {
      throw new Error("EACCES: permission denied, access '/dev/kvm'");
    });

    try {
      validateEnvironment("/fake/base");
      expect.unreachable("should have thrown");
    } catch (err: unknown) {
      const error = err as { code: string };
      expect(error.code).toBe("ERR_SETUP_KVM_UNAVAILABLE");
    }
  });
});
