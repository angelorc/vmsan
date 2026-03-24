import { consola } from "consola";
import { toError } from "./utils.ts";

export interface HostInfo {
  id: string;
  name: string;
  address: string;
  status: string;
  vm_count: number;
  resources?: { cpus: number; memory_mb: number; disk_gb: number };
  last_heartbeat?: string;
}

export interface CreateVMRequest {
  name?: string;
  host_id: string;
  project?: string;
  service?: string;
  state: Record<string, unknown>;
}

export class ServerClient {
  constructor(private baseUrl: string) {}

  static fromEnv(): ServerClient {
    const url = process.env.VMSAN_SERVER_URL ?? "http://10.88.0.1:6443";
    return new ServerClient(url);
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    consola.debug(`${method} ${url}`);

    const res = await fetch(url, {
      method,
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    });

    if (!res.ok) {
      const text = await res.text().catch(() => "");
      let message = `Server returned ${res.status}`;
      try {
        const json = JSON.parse(text);
        if (json.error) message = json.error;
      } catch {
        if (text) message = text;
      }
      throw new Error(message);
    }

    const contentType = res.headers.get("content-type") ?? "";
    if (contentType.includes("application/json")) {
      return (await res.json()) as T;
    }
    return undefined as T;
  }

  async status(): Promise<{ ok: boolean; version: string; hosts: number; vms: number }> {
    return this.request("GET", "/api/v1/status");
  }

  async listHosts(): Promise<HostInfo[]> {
    return this.request("GET", "/api/v1/hosts");
  }

  async getHost(id: string): Promise<HostInfo | null> {
    try {
      return await this.request<HostInfo>("GET", `/api/v1/hosts/${encodeURIComponent(id)}`);
    } catch (err) {
      consola.debug(`getHost failed: ${toError(err).message}`);
      return null;
    }
  }

  async findHostByName(name: string): Promise<HostInfo | null> {
    const hosts = await this.listHosts();
    return hosts.find((h) => h.name === name) ?? null;
  }

  async deleteHost(id: string): Promise<void> {
    await this.request<void>("DELETE", `/api/v1/hosts/${encodeURIComponent(id)}`);
  }

  async generateToken(): Promise<{ token: string; expires_at: string }> {
    return this.request("POST", "/api/v1/tokens");
  }

  async createVM(opts: CreateVMRequest): Promise<{ id: string }> {
    return this.request("POST", "/api/v1/vms", opts);
  }

}
