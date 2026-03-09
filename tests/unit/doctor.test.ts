import { describe, test, expect, vi, beforeEach } from "vitest";
import { accessSync, existsSync, readdirSync, statfsSync } from "node:fs";
import { execSync } from "node:child_process";

vi.mock("node:fs", async (importOriginal) => {
  const actual = await importOriginal<typeof import("node:fs")>();
  return {
    ...actual,
    accessSync: vi.fn(),
    existsSync: vi.fn(),
    readdirSync: vi.fn(),
    statfsSync: vi.fn(),
  };
});

vi.mock("node:child_process", async (importOriginal) => {
  const actual = await importOriginal<typeof import("node:child_process")>();
  return {
    ...actual,
    execSync: vi.fn(),
  };
});

import { runDoctorChecks } from "../../src/commands/doctor.ts";
import type { VmsanPaths } from "../../src/paths.ts";

const fakePaths: VmsanPaths = {
  baseDir: "/fake/.vmsan",
  vmsDir: "/fake/.vmsan/vms",
  jailerBaseDir: "/fake/.vmsan/jailer",
  binDir: "/fake/.vmsan/bin",
  agentBin: "/fake/.vmsan/bin/vmsan-agent",
  kernelsDir: "/fake/.vmsan/kernels",
  rootfsDir: "/fake/.vmsan/rootfs",
  registryDir: "/fake/.vmsan/registry/rootfs",
  snapshotsDir: "/fake/.vmsan/snapshots",
  seccompDir: "/fake/.vmsan/seccomp",
  seccompFilter: "/fake/.vmsan/seccomp/default.json",
  agentPort: 9119,
};

describe("runDoctorChecks", () => {
  beforeEach(() => {
    vi.mocked(accessSync).mockReset();
    vi.mocked(existsSync).mockReset();
    vi.mocked(readdirSync).mockReset();
    vi.mocked(statfsSync).mockReset();
    vi.mocked(execSync).mockReset();
  });

  function setupAllPassing(): void {
    // KVM accessible
    vi.mocked(accessSync).mockImplementation(() => {});

    // Disk space: 50 GB free
    vi.mocked(statfsSync).mockReturnValue({
      bfree: 12_500_000,
      bsize: 4096,
    } as ReturnType<typeof statfsSync>);

    // Default interface
    vi.mocked(execSync).mockImplementation((cmd: string | URL) => {
      const cmdStr = String(cmd);
      if (cmdStr.includes("ip route show default")) {
        return "default via 192.168.1.1 dev eth0 proto dhcp metric 100\n";
      }
      if (cmdStr.includes("--version")) {
        return "firecracker v1.14.2\n";
      }
      return "";
    });

    // existsSync: firecracker, jailer, agent, rootfs all exist
    vi.mocked(existsSync).mockImplementation((p) => {
      const path = String(p);
      if (path.includes("firecracker")) return true;
      if (path.includes("jailer")) return true;
      if (path.includes("vmsan-agent")) return true;
      if (path.includes("kernels")) return true;
      if (path.includes("ubuntu-24.04.ext4")) return true;
      return false;
    });

    // Kernel dir has files
    vi.mocked(readdirSync).mockReturnValue(["vmlinux-6.1.155"] as unknown as ReturnType<
      typeof readdirSync
    >);
  }

  test("all checks pass when environment is healthy", () => {
    setupAllPassing();
    const checks = runDoctorChecks(fakePaths);

    expect(checks).toHaveLength(8);
    expect(checks.every((c) => c.status === "pass")).toBe(true);

    const summary = {
      passed: checks.filter((c) => c.status === "pass").length,
      failed: checks.filter((c) => c.status === "fail").length,
    };
    expect(summary).toEqual({ passed: 8, failed: 0 });
  });

  test("KVM check fails when /dev/kvm is not accessible", () => {
    setupAllPassing();
    vi.mocked(accessSync).mockImplementation(() => {
      throw new Error("EACCES");
    });

    const checks = runDoctorChecks(fakePaths);
    const kvmCheck = checks.find((c) => c.name === "KVM")!;
    expect(kvmCheck.status).toBe("fail");
    expect(kvmCheck.detail).toBe("KVM not available");
  });

  test("disk space check fails when less than 5 GB free", () => {
    setupAllPassing();
    vi.mocked(statfsSync).mockReturnValue({
      bfree: 500_000,
      bsize: 4096,
    } as ReturnType<typeof statfsSync>);

    const checks = runDoctorChecks(fakePaths);
    const diskCheck = checks.find((c) => c.name === "Disk space")!;
    expect(diskCheck.status).toBe("fail");
    expect(diskCheck.detail).toContain("Low disk space");
  });

  test("default interface check fails when no route found", () => {
    setupAllPassing();
    vi.mocked(execSync).mockImplementation((cmd: string | URL) => {
      const cmdStr = String(cmd);
      if (cmdStr.includes("ip route show default")) {
        throw new Error("Command failed");
      }
      if (cmdStr.includes("--version")) {
        return "firecracker v1.14.2\n";
      }
      return "";
    });

    const checks = runDoctorChecks(fakePaths);
    const ifaceCheck = checks.find((c) => c.name === "Default interface")!;
    expect(ifaceCheck.status).toBe("fail");
    expect(ifaceCheck.detail).toBe("No default route");
  });

  test("firecracker check fails when binary missing", () => {
    setupAllPassing();
    vi.mocked(existsSync).mockImplementation((p) => {
      const path = String(p);
      if (path.includes("firecracker")) return false;
      if (path.includes("jailer")) return true;
      if (path.includes("vmsan-agent")) return true;
      if (path.includes("kernels")) return true;
      if (path.includes("ubuntu-24.04.ext4")) return true;
      return false;
    });

    const checks = runDoctorChecks(fakePaths);
    const fcCheck = checks.find((c) => c.name === "Firecracker")!;
    expect(fcCheck.status).toBe("fail");
    expect(fcCheck.detail).toBe("Not found");
  });

  test("jailer check fails when binary missing", () => {
    setupAllPassing();
    vi.mocked(existsSync).mockImplementation((p) => {
      const path = String(p);
      if (path.includes("firecracker") && !path.includes("jailer")) return true;
      if (path.includes("jailer")) return false;
      if (path.includes("vmsan-agent")) return true;
      if (path.includes("kernels")) return true;
      if (path.includes("ubuntu-24.04.ext4")) return true;
      return false;
    });

    const checks = runDoctorChecks(fakePaths);
    const jailerCheck = checks.find((c) => c.name === "Jailer")!;
    expect(jailerCheck.status).toBe("fail");
    expect(jailerCheck.detail).toBe("Not found");
  });

  test("agent check fails when binary missing", () => {
    setupAllPassing();
    vi.mocked(existsSync).mockImplementation((p) => {
      const path = String(p);
      if (path.includes("firecracker")) return true;
      if (path.includes("jailer")) return true;
      if (path.includes("vmsan-agent")) return false;
      if (path.includes("kernels")) return true;
      if (path.includes("ubuntu-24.04.ext4")) return true;
      return false;
    });

    const checks = runDoctorChecks(fakePaths);
    const agentCheck = checks.find((c) => c.name === "Agent")!;
    expect(agentCheck.status).toBe("fail");
    expect(agentCheck.detail).toBe("Not found");
  });

  test("kernel check fails when no vmlinux files found", () => {
    setupAllPassing();
    vi.mocked(readdirSync).mockReturnValue([] as unknown as ReturnType<typeof readdirSync>);

    const checks = runDoctorChecks(fakePaths);
    const kernelCheck = checks.find((c) => c.name === "Kernel")!;
    expect(kernelCheck.status).toBe("fail");
    expect(kernelCheck.detail).toBe("Not found");
  });

  test("rootfs check fails when ubuntu-24.04.ext4 missing", () => {
    setupAllPassing();
    vi.mocked(existsSync).mockImplementation((p) => {
      const path = String(p);
      if (path.includes("ubuntu-24.04.ext4")) return false;
      if (path.includes("firecracker")) return true;
      if (path.includes("jailer")) return true;
      if (path.includes("vmsan-agent")) return true;
      if (path.includes("kernels")) return true;
      return false;
    });

    const checks = runDoctorChecks(fakePaths);
    const rootfsCheck = checks.find((c) => c.name === "Rootfs (base)")!;
    expect(rootfsCheck.status).toBe("fail");
    expect(rootfsCheck.detail).toBe("Not found");
  });

  test("all checks run even when some fail", () => {
    // Everything fails
    vi.mocked(accessSync).mockImplementation(() => {
      throw new Error("EACCES");
    });
    vi.mocked(statfsSync).mockImplementation(() => {
      throw new Error("ENOENT");
    });
    vi.mocked(execSync).mockImplementation(() => {
      throw new Error("Command failed");
    });
    vi.mocked(existsSync).mockReturnValue(false);
    vi.mocked(readdirSync).mockReturnValue([] as unknown as ReturnType<typeof readdirSync>);

    const checks = runDoctorChecks(fakePaths);

    // All 8 checks should still run
    expect(checks).toHaveLength(8);
    expect(checks.every((c) => c.status === "fail")).toBe(true);

    const summary = {
      passed: checks.filter((c) => c.status === "pass").length,
      failed: checks.filter((c) => c.status === "fail").length,
    };
    expect(summary).toEqual({ passed: 0, failed: 8 });
  });

  test("summary counts are correct with mixed results", () => {
    setupAllPassing();
    // Break only KVM and disk
    vi.mocked(accessSync).mockImplementation(() => {
      throw new Error("EACCES");
    });
    vi.mocked(statfsSync).mockReturnValue({
      bfree: 100,
      bsize: 4096,
    } as ReturnType<typeof statfsSync>);

    const checks = runDoctorChecks(fakePaths);
    const passed = checks.filter((c) => c.status === "pass").length;
    const failed = checks.filter((c) => c.status === "fail").length;

    expect(passed).toBe(6);
    expect(failed).toBe(2);
    expect(passed + failed).toBe(8);
  });

  test("checks are categorized correctly", () => {
    setupAllPassing();
    const checks = runDoctorChecks(fakePaths);

    const systemChecks = checks.filter((c) => c.category === "System");
    const binaryChecks = checks.filter((c) => c.category === "Binaries");
    const imageChecks = checks.filter((c) => c.category === "Images");

    expect(systemChecks).toHaveLength(3);
    expect(binaryChecks).toHaveLength(3);
    expect(imageChecks).toHaveLength(2);
  });

  test("firecracker version is included in pass detail", () => {
    setupAllPassing();
    const checks = runDoctorChecks(fakePaths);
    const fcCheck = checks.find((c) => c.name === "Firecracker")!;
    expect(fcCheck.status).toBe("pass");
    expect(fcCheck.detail).toContain("firecracker v1.14.2");
  });

  test("kernel filename is included in pass detail", () => {
    setupAllPassing();
    const checks = runDoctorChecks(fakePaths);
    const kernelCheck = checks.find((c) => c.name === "Kernel")!;
    expect(kernelCheck.status).toBe("pass");
    expect(kernelCheck.detail).toBe("vmlinux-6.1.155");
  });

  test("disk space shows GB in pass detail", () => {
    setupAllPassing();
    const checks = runDoctorChecks(fakePaths);
    const diskCheck = checks.find((c) => c.name === "Disk space")!;
    expect(diskCheck.status).toBe("pass");
    expect(diskCheck.detail).toMatch(/\d+ GB free/);
  });

  test("default interface name is included in pass detail", () => {
    setupAllPassing();
    const checks = runDoctorChecks(fakePaths);
    const ifaceCheck = checks.find((c) => c.name === "Default interface")!;
    expect(ifaceCheck.status).toBe("pass");
    expect(ifaceCheck.detail).toBe("eth0");
  });
});
