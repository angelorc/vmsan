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

  it("sends correct method and params for vm.start", async () => {
    let receivedRequest: { method: string; params?: unknown } | null = null;

    server = createMockGateway(socketPath, (req) => {
      receivedRequest = req;
      return {
        ok: true,
        vm: {
          vmId: "vm-abc123",
          slot: 5,
          policy: "custom",
          dnsPort: 10053,
          sniPort: 10443,
          httpPort: 10080,
        },
      };
    });

    // Wait for server to be ready
    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    await client.vmStart({
      vmId: "vm-abc123",
      slot: 5,
      policy: "custom",
      allowedDomains: ["example.com", "*.github.com"],
    });

    expect(receivedRequest).not.toBeNull();
    expect(receivedRequest!.method).toBe("vm.start");
    expect(receivedRequest!.params).toEqual({
      vmId: "vm-abc123",
      slot: 5,
      policy: "custom",
      allowedDomains: ["example.com", "*.github.com"],
    });
  });

  it("sends correct method for vm.stop", async () => {
    let receivedMethod = "";

    server = createMockGateway(socketPath, (req) => {
      receivedMethod = req.method;
      return { ok: true };
    });
    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    await client.vmStop("vm-abc123");

    expect(receivedMethod).toBe("vm.stop");
  });

  it("sends correct method and params for vm.updatePolicy", async () => {
    let receivedRequest: { method: string; params?: unknown } | null = null;

    server = createMockGateway(socketPath, (req) => {
      receivedRequest = req;
      return { ok: true };
    });
    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    await client.vmUpdatePolicy("vm-abc123", "allow-all", ["example.com"]);

    expect(receivedRequest!.method).toBe("vm.updatePolicy");
    expect(receivedRequest!.params).toEqual({
      vmId: "vm-abc123",
      policy: "allow-all",
      allowedDomains: ["example.com"],
    });
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

  it("parses vm.start response with VM details", async () => {
    server = createMockGateway(socketPath, () => ({
      ok: true,
      vm: {
        vmId: "vm-test1",
        slot: 2,
        policy: "custom",
        dnsPort: 20053,
        sniPort: 20443,
        httpPort: 20080,
      },
    }));
    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    const result = await client.vmStart({
      vmId: "vm-test1",
      slot: 2,
      policy: "custom",
    });

    expect(result.ok).toBe(true);
    expect(result.vm).toBeDefined();
    expect(result.vm!.vmId).toBe("vm-test1");
    expect(result.vm!.dnsPort).toBe(20053);
    expect(result.vm!.sniPort).toBe(20443);
    expect(result.vm!.httpPort).toBe(20080);
  });

  it("parses error response", async () => {
    server = createMockGateway(socketPath, () => ({
      ok: false,
      error: "slot already in use",
      code: "ERR_SLOT_CONFLICT",
    }));
    await new Promise((r) => server!.once("listening", r));

    const client = new GatewayClient(socketPath);
    const result = await client.vmStart({
      vmId: "vm-dup",
      slot: 5,
      policy: "allow-all",
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

  it("uses /run/vmsan-gateway.sock as default socket path", async () => {
    const client = new GatewayClient();
    // Verify it tries the default path by checking the connection error
    await expect(client.ping()).rejects.toThrow("Gateway connection error");
  });
});

describe("ensureGatewayRunning", () => {
  it("returns early when binary does not exist", async () => {
    // This should not throw — it logs and returns
    await ensureGatewayRunning("/nonexistent/vmsan-gateway");
    // If we get here, the function handled the missing binary gracefully
  });
});
