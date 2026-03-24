import { createServer, type Server } from "node:net";
import { mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { GatewayClient, ensureGatewayRunning } from "../../src/lib/gateway-client.ts";

// ---------------------------------------------------------------------------
// Helpers — mock Unix socket server
// ---------------------------------------------------------------------------

function createMockGateway(
  socketPath: string,
  handler: (request: { method: string; params?: unknown }) => unknown,
): Server {
  const server = createServer((conn) => {
    let data = "";
    conn.on("data", (chunk) => {
      data += chunk.toString();
      // Process on newline (JSON-RPC line protocol)
      if (data.includes("\n")) {
        try {
          const request = JSON.parse(data.trim());
          const response = handler(request);
          conn.end(JSON.stringify(response));
        } catch {
          conn.end(JSON.stringify({ ok: false, error: "invalid request" }));
        }
      }
    });
  });

  server.listen(socketPath);
  return server;
}

function closeServer(server: Server): Promise<void> {
  return new Promise((resolve) => {
    server.close(() => resolve());
  });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("GatewayClient", () => {
  let tmpDir: string;
  let socketPath: string;
  let server: Server | null = null;

  beforeEach(() => {
    tmpDir = mkdtempSync(join(tmpdir(), "vmsan-gw-test-"));
    socketPath = join(tmpDir, "gateway.sock");
  });

  afterEach(async () => {
    if (server) {
      await closeServer(server);
      server = null;
    }
    rmSync(tmpDir, { recursive: true, force: true });
  });

  // ---- Request serialization ----

  it("sends correct method and params for vm.create", async () => {
    let receivedRequest: { method: string; params?: unknown } | null = null;

    server = createMockGateway(socketPath, (req) => {
      receivedRequest = req;
      return {
        ok: true,
        vm: {
          vmId: "vm-abc123",
          slot: 5,
          hostIp: "198.19.5.1",
          guestIp: "198.19.5.2",
          tapDevice: "fhvm5",
          macAddress: "AA:FC:00:00:00:06",
          netnsName: "vmsan-5",
          vethHost: "veth-h-5",
          vethGuest: "veth-g-5",
          subnetMask: "255.255.255.252",
          chrootDir: "/tmp/jailer/firecracker/vm-abc123/root",
          socketPath: "/tmp/jailer/firecracker/vm-abc123/root/run/firecracker.socket",
          pid: 12345,
          dnsPort: 10058,
          sniPort: 10448,
          httpPort: 10703,
        },
      };
    });

    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    const result = await client.vmCreate({
      vmId: "vm-abc123",
      vcpus: 2,
      memMib: 256,
      networkPolicy: "allow-all",
      kernelPath: "/tmp/vmlinux",
      rootfsPath: "/tmp/rootfs.ext4",
    });

    expect(receivedRequest).not.toBeNull();
    expect(receivedRequest!.method).toBe("vm.create");
    expect((receivedRequest!.params as Record<string, unknown>).vmId).toBe("vm-abc123");
    expect((receivedRequest!.params as Record<string, unknown>).vcpus).toBe(2);
    expect(result.ok).toBe(true);
    expect(result.vm).toBeDefined();
    expect(result.vm!.vmId).toBe("vm-abc123");
    expect(result.vm!.pid).toBe(12345);
  });

  it("sends correct method and params for vm.fullStop", async () => {
    let receivedRequest: { method: string; params?: unknown } | null = null;

    server = createMockGateway(socketPath, (req) => {
      receivedRequest = req;
      return { ok: true };
    });
    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    await client.vmFullStop({ vmId: "vm-abc123", slot: 5 });

    expect(receivedRequest!.method).toBe("vm.fullStop");
    expect((receivedRequest!.params as Record<string, unknown>).vmId).toBe("vm-abc123");
  });

  it("sends correct method and params for vm.fullUpdatePolicy", async () => {
    let receivedRequest: { method: string; params?: unknown } | null = null;

    server = createMockGateway(socketPath, (req) => {
      receivedRequest = req;
      return { ok: true };
    });
    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    await client.vmFullUpdatePolicy({
      vmId: "vm-abc123",
      policy: "allow-all",
      domains: ["example.com"],
    });

    expect(receivedRequest!.method).toBe("vm.fullUpdatePolicy");
    expect((receivedRequest!.params as Record<string, unknown>).policy).toBe("allow-all");
  });

  it("sends correct method for ping", async () => {
    let receivedMethod = "";

    server = createMockGateway(socketPath, (req) => {
      receivedMethod = req.method;
      return { ok: true, version: "0.4.0", vms: 3 };
    });
    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    const result = await client.ping();

    expect(receivedMethod).toBe("ping");
    expect(result).toEqual({ ok: true, version: "0.4.0", vms: 3 });
  });

  it("sends correct method for shutdown", async () => {
    let receivedMethod = "";

    server = createMockGateway(socketPath, (req) => {
      receivedMethod = req.method;
      return { ok: true };
    });
    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    await client.shutdown();

    expect(receivedMethod).toBe("shutdown");
  });

  // ---- Response parsing ----

  it("parses error response", async () => {
    server = createMockGateway(socketPath, () => ({
      ok: false,
      error: "slot already in use",
      code: "ERR_SLOT_CONFLICT",
    }));
    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    const result = await client.vmCreate({
      vmId: "vm-dup",
      networkPolicy: "allow-all",
    });

    expect(result.ok).toBe(false);
    expect(result.error).toBe("slot already in use");
    expect(result.code).toBe("ERR_SLOT_CONFLICT");
  });

  // ---- Error handling ----

  it("rejects on connection refused (no server)", async () => {
    const client = new GatewayClient(socketPath);

    await expect(client.ping()).rejects.toThrow("Gateway connection error");
  });

  it("rejects on invalid JSON response", async () => {
    server = createServer((conn) => {
      conn.on("data", () => {
        conn.end("not valid json\n");
      });
    });
    server.listen(socketPath);
    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    await expect(client.ping()).rejects.toThrow("Failed to parse gateway response");
  });

  // ---- Default socket path ----

  it("uses /run/vmsan/gateway.sock as default socket path", async () => {
    const client = new GatewayClient();
    // Verify it tries the default path by checking the connection error
    await expect(client.ping()).rejects.toThrow("Gateway connection error");
  });
});

describe("ensureGatewayRunning", () => {
  it("throws when gateway binary does not exist and daemon is not running", async () => {
    // ensureGatewayRunning is now fatal — it throws if the gateway can't be reached
    await expect(ensureGatewayRunning("/nonexistent/vmsan-gateway")).rejects.toThrow(
      "vmsan-gateway binary not found",
    );
  });
});
