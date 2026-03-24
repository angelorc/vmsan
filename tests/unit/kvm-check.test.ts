import { describe, test, expect, vi, beforeEach } from "vitest";
import { existsSync } from "node:fs";
import { validateEnvironment } from "../../src/commands/create/environment.ts";

vi.mock("node:fs", async (importOriginal) => {
  const actual = await importOriginal<typeof import("node:fs")>();
  return {
    ...actual,
    existsSync: vi.fn(),
  };
});

describe("validateEnvironment", () => {
  beforeEach(() => {
    vi.mocked(existsSync).mockReset();
  });

  test("does not check KVM directly — gateway handles KVM validation", () => {
    // Firecracker and Jailer binaries exist
    vi.mocked(existsSync).mockReturnValue(true);

    // Should not throw — KVM access is now checked by the gateway doctor RPC
    expect(() => validateEnvironment("/fake/base")).not.toThrow();
  });

  test("throws when firecracker binary is missing", () => {
    vi.mocked(existsSync).mockReturnValue(false);

    expect(() => validateEnvironment("/fake/base")).toThrow();
  });
});
