import { connect } from "node:net";
import { existsSync } from "node:fs";
import { spawn } from "node:child_process";
import { consola } from "consola";

export interface GatewayVmConfig {
  vmId: string;
  slot: number;
  policy: string;
  allowedDomains?: string[];
  project?: string;
  service?: string;
  connectTo?: string[];
}

export interface GatewayVmResult {
  ok: boolean;
  error?: string;
  code?: string;
  vm?: {
    vmId: string;
    slot: number;
    policy: string;
    dnsPort: number;
    sniPort: number;
    httpPort: number;
  };
}

export interface GatewayPingResult {
  ok: boolean;
  version: string;
  vms: number;
}

export class GatewayClient {
  constructor(private socketPath: string = "/run/vmsan-gateway.sock") {}

  async vmStart(config: GatewayVmConfig): Promise<GatewayVmResult> {
    return this.send("vm.start", config);
  }

  async vmStop(vmId: string): Promise<void> {
    await this.send("vm.stop", { vmId });
  }

  async vmUpdatePolicy(vmId: string, policy: string, allowedDomains?: string[]): Promise<void> {
    await this.send("vm.updatePolicy", { vmId, policy, allowedDomains });
  }

  async ping(): Promise<GatewayPingResult> {
    return this.send("ping");
  }

  async shutdown(): Promise<void> {
    await this.send("shutdown");
  }

  private async send(method: string, params?: unknown): Promise<any> {
    return new Promise((resolve, reject) => {
      const socket = connect(this.socketPath);
      let data = "";

      socket.on("connect", () => {
        const request = JSON.stringify({ method, params });
        socket.write(request + "\n");
      });

      socket.on("data", (chunk) => {
        data += chunk.toString();
      });

      socket.on("end", () => {
        try {
          const response = JSON.parse(data);
          resolve(response);
        } catch {
          reject(new Error(`Failed to parse gateway response: ${data}`));
        }
      });

      socket.on("error", (err) => {
        reject(new Error(`Gateway connection error: ${err.message}`));
      });

      // Timeout after 10 seconds
      socket.setTimeout(10000);
      socket.on("timeout", () => {
        socket.destroy();
        reject(new Error("Gateway connection timeout"));
      });
    });
  }
}

/**
 * Ensure vmsan-gateway is running. If not, start it in the background
 * and wait for it to become ready. Non-fatal — if the gateway can't
 * start, we log and continue without proxy support.
 */
export async function ensureGatewayRunning(gatewayBin: string): Promise<void> {
  const client = new GatewayClient();
  try {
    await client.ping();
    return; // Already running
  } catch {
    // Not running, start it
  }

  if (!existsSync(gatewayBin)) {
    consola.debug("vmsan-gateway binary not found, skipping proxy layer");
    return;
  }

  // Start gateway in background
  const child = spawn(gatewayBin, ["start"], {
    detached: true,
    stdio: "ignore",
  });
  child.unref();

  // Wait for it to be ready (up to 2 seconds)
  for (let i = 0; i < 10; i++) {
    await new Promise((r) => setTimeout(r, 200));
    try {
      await client.ping();
      return;
    } catch {
      // Still starting
    }
  }
  consola.debug("vmsan-gateway failed to start within 2 seconds");
}
