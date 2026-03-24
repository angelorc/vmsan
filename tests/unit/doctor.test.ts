import { describe, test, expect, vi, beforeEach } from "vitest";

// Mock the gateway-client module with a proper class
const mockDoctor = vi.fn();
vi.mock("../../src/lib/gateway-client.ts", () => ({
  GatewayClient: class {
    doctor = mockDoctor;
  },
}));

import { runDoctorChecks } from "../../src/commands/doctor.ts";

describe("runDoctorChecks", () => {
  beforeEach(() => {
    mockDoctor.mockReset();
  });

  test("returns checks from gateway doctor RPC", async () => {
    mockDoctor.mockResolvedValue({
      ok: true,
      list: [
        { category: "virtualization", name: "KVM access", status: "pass", detail: "/dev/kvm is accessible" },
        { category: "networking", name: "TUN device", status: "pass", detail: "/dev/net/tun is accessible" },
        { category: "storage", name: "disk space", status: "pass", detail: "50.0 GB free on /srv/jailer" },
      ],
    });

    const checks = await runDoctorChecks();
    expect(checks).toHaveLength(3);
    expect(checks[0].name).toBe("KVM access");
    expect(checks[0].status).toBe("pass");
    expect(checks[1].name).toBe("TUN device");
    expect(checks[2].name).toBe("disk space");
  });

  test("maps gateway check fields correctly", async () => {
    mockDoctor.mockResolvedValue({
      ok: true,
      list: [
        {
          category: "virtualization",
          name: "KVM access",
          status: "fail",
          detail: "/dev/kvm not accessible",
          fix: "Enable KVM in BIOS",
        },
      ],
    });

    const checks = await runDoctorChecks();
    expect(checks[0]).toEqual({
      category: "virtualization",
      name: "KVM access",
      status: "fail",
      detail: "/dev/kvm not accessible",
      fix: "Enable KVM in BIOS",
    });
  });

  test("handles warn status from gateway", async () => {
    mockDoctor.mockResolvedValue({
      ok: true,
      list: [
        { category: "firewall", name: "host firewall", status: "warn", detail: "ufw is active" },
      ],
    });

    const checks = await runDoctorChecks();
    expect(checks[0].status).toBe("warn");
  });

  test("throws when gateway returns error", async () => {
    mockDoctor.mockResolvedValue({
      ok: false,
      error: "gateway not running",
    });

    await expect(runDoctorChecks()).rejects.toThrow("gateway not running");
  });

  test("throws when gateway connection fails", async () => {
    mockDoctor.mockRejectedValue(new Error("connection refused"));

    await expect(runDoctorChecks()).rejects.toThrow("connection refused");
  });

  test("returns empty array when gateway returns empty list", async () => {
    mockDoctor.mockResolvedValue({
      ok: true,
      list: [],
    });

    const checks = await runDoctorChecks();
    expect(checks).toHaveLength(0);
  });

  test("handles null list from gateway", async () => {
    mockDoctor.mockResolvedValue({
      ok: true,
      list: null,
    });

    const checks = await runDoctorChecks();
    expect(checks).toHaveLength(0);
  });

  test("returns all check categories from gateway", async () => {
    mockDoctor.mockResolvedValue({
      ok: true,
      list: [
        { category: "virtualization", name: "KVM access", status: "pass", detail: "ok" },
        { category: "networking", name: "TUN device", status: "pass", detail: "ok" },
        { category: "storage", name: "disk space", status: "pass", detail: "ok" },
        { category: "firewall", name: "nftables", status: "pass", detail: "ok" },
        { category: "binaries", name: "firecracker", status: "pass", detail: "/usr/bin/firecracker" },
        { category: "binaries", name: "jailer", status: "pass", detail: "/usr/bin/jailer" },
        { category: "images", name: "kernel image", status: "pass", detail: "vmlinux-5.10" },
        { category: "images", name: "rootfs image", status: "pass", detail: "ubuntu-24.04.ext4" },
        { category: "daemon", name: "gateway process", status: "pass", detail: "running" },
      ],
    });

    const checks = await runDoctorChecks();
    expect(checks).toHaveLength(9);
    const categories = [...new Set(checks.map((c) => c.category))];
    expect(categories).toContain("virtualization");
    expect(categories).toContain("binaries");
    expect(categories).toContain("daemon");
  });
});
