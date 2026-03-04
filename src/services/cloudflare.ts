import { randomBytes } from "node:crypto";
import { closeSync, existsSync, openSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { spawn } from "node:child_process";
import Cloudflare from "cloudflare";
import { consola } from "consola";
import pRetry, { AbortError } from "p-retry";
import { mkdirSecure, sleepSync, toError, writeSecure } from "../lib/utils.ts";
import { FileLock } from "../lib/file-lock.ts";
import { PidFile } from "../lib/pid-file.ts";
import {
  cloudflareNotConfiguredError,
  cloudflareTunnelNoIdError,
  cloudflaredNotFoundError,
  cloudflareConfigNotFoundError,
  cloudflaredStartFailedError,
  cloudflareNoAccountsError,
  cloudflareNoZoneError,
} from "../errors/index.ts";

export interface CloudflareConfig {
  token: string;
  domain: string;
  tunnelId?: string;
  accountId?: string;
  tunnelToken?: string;
}

export interface TunnelRoute {
  vmId: string;
  hostname: string;
  service: string;
}

export class CloudflareService {
  private readonly cfDir: string;
  private readonly configPath: string;
  private readonly routesPath: string;
  private readonly logPath: string;
  private readonly cloudflaredBin: string;
  private readonly lock: FileLock;
  private readonly pidFile: PidFile;

  constructor(baseDir: string) {
    this.cfDir = join(baseDir, "cloudflare");
    this.configPath = join(this.cfDir, "cloudflare.json");
    this.routesPath = join(this.cfDir, "routes.json");
    this.logPath = join(this.cfDir, "cloudflared.log");
    this.cloudflaredBin = join(baseDir, "bin", "cloudflared");
    this.lock = new FileLock(join(this.cfDir, "lock"), "Cloudflare");
    this.pidFile = new PidFile(join(this.cfDir, "cloudflared.pid"));
  }

  private getClient(token: string): Cloudflare {
    return new Cloudflare({ apiToken: token });
  }

  load(): CloudflareConfig | null {
    if (!existsSync(this.configPath)) return null;
    try {
      return JSON.parse(readFileSync(this.configPath, "utf-8")) as CloudflareConfig;
    } catch (err) {
      consola.warn(`Cloudflare config file corrupt: ${toError(err).message}`);
      return null;
    }
  }

  save(config: CloudflareConfig): void {
    mkdirSecure(this.cfDir);
    writeSecure(this.configPath, JSON.stringify(config, null, 2));
  }

  isConfigured(): boolean {
    if (process.env.VMSAN_DISABLE_CLOUDFLARE === "1") {
      return false;
    }
    const config = this.load();
    return config !== null && !!config.token && !!config.domain;
  }

  isInstalled(): boolean {
    return existsSync(this.cloudflaredBin);
  }

  isRunning(): boolean {
    return this.pidFile.read() !== null;
  }

  getHostnames(vmId: string): string[] {
    return this.loadRoutes()
      .filter((route) => route.vmId === vmId)
      .map((route) => route.hostname);
  }

  async createTunnel(): Promise<{ tunnelId: string }> {
    return this.lock.runAsync(async () => {
      const config = this.load();
      if (!config) throw cloudflareNotConfiguredError();

      const previousTunnelId = config.tunnelId;

      if (config.tunnelId && config.tunnelToken) {
        return { tunnelId: config.tunnelId };
      }

      const client = this.getClient(config.token);
      const accountId = config.accountId || (await this.resolveAccountId(client, config.domain));
      const tunnelSecret = randomBytes(32).toString("base64");
      const tunnelName =
        config.tunnelId && !config.tunnelToken ? `vmsan-${Date.now().toString(36)}` : "vmsan";

      let tunnel: { id?: string; token?: string };
      try {
        tunnel = await client.zeroTrust.tunnels.cloudflared.create({
          account_id: accountId,
          name: tunnelName,
          tunnel_secret: tunnelSecret,
          config_src: "cloudflare",
        });
      } catch (err) {
        const errMsg = toError(err).message;
        if (errMsg.includes("409") || errMsg.toLowerCase().includes("conflict")) {
          consola.debug(`Tunnel name "${tunnelName}" already exists, cleaning up stale tunnel...`);
          await this.deleteExistingTunnel(client, accountId, tunnelName);
          tunnel = await client.zeroTrust.tunnels.cloudflared.create({
            account_id: accountId,
            name: tunnelName,
            tunnel_secret: tunnelSecret,
            config_src: "cloudflare",
          });
        } else {
          throw err;
        }
      }

      const tunnelId = tunnel.id;
      if (!tunnelId) throw cloudflareTunnelNoIdError();

      const tunnelToken = (tunnel as Record<string, unknown>).token as string | undefined;

      config.tunnelId = tunnelId;
      config.accountId = accountId;
      config.tunnelToken = tunnelToken;
      this.save(config);

      if (previousTunnelId && previousTunnelId !== tunnelId) {
        try {
          await client.zeroTrust.tunnels.cloudflared.delete(previousTunnelId, {
            account_id: accountId,
          });
        } catch (err) {
          consola.debug(
            `Failed to delete previous tunnel ${previousTunnelId}: ${toError(err).message}`,
          );
        }
      }

      return { tunnelId };
    });
  }

  async pushConfig(): Promise<void> {
    const config = this.load();
    if (!config?.tunnelId) return;

    const client = this.getClient(config.token);
    const accountId = config.accountId || (await this.resolveAccountId(client, config.domain));
    if (!config.accountId) {
      config.accountId = accountId;
      this.save(config);
    }

    const ingress = this.buildIngress(this.loadRoutes());
    await client.zeroTrust.tunnels.cloudflared.configurations.update(config.tunnelId, {
      account_id: accountId,
      config: { ingress: ingress as Array<{ hostname: string; service: string }> },
    });
  }

  async pushConfigWithRetry(maxRetries = 3): Promise<void> {
    await pRetry(() => this.pushConfig(), {
      retries: maxRetries,
      minTimeout: 1000,
      factor: 2,
      onFailedAttempt: (ctx) => {
        if (ctx.error.message?.includes("403") || ctx.error.message?.includes("401")) {
          throw new AbortError(ctx.error);
        }
      },
    });
  }

  addRoute(route: TunnelRoute): void {
    this.addRoutes([route]);
  }

  addRoutes(newRoutes: TunnelRoute[]): void {
    if (newRoutes.length === 0) return;
    this.lock.run(() => {
      let routes = this.loadRoutes();
      for (const route of newRoutes) {
        routes = routes.filter((r) => r.vmId !== route.vmId || r.hostname !== route.hostname);
        routes.push(route);
      }
      this.saveRoutes(routes);
    });
  }

  removeRoute(vmId: string): void {
    this.lock.run(() => {
      const routes = this.loadRoutes();
      const filtered = routes.filter((r) => r.vmId !== vmId);
      this.saveRoutes(filtered);
    });
  }

  async addDns(hostname: string, tunnelId: string): Promise<void> {
    const config = this.load();
    if (!config) throw cloudflareNotConfiguredError();

    const client = this.getClient(config.token);
    const zoneId = await this.resolveZoneId(client, config.domain);

    const record = {
      zone_id: zoneId,
      type: "CNAME" as const,
      name: hostname,
      content: `${tunnelId}.cfargotunnel.com`,
      proxied: true,
      ttl: 1 as const,
    };

    const existing = await client.dns.records.list({
      zone_id: zoneId,
      type: "CNAME",
      name: { exact: hostname },
    });
    const records = existing.getPaginatedItems();
    if (records.length > 0) {
      await client.dns.records.update(records[0].id, record);
      return;
    }

    await client.dns.records.create(record);
  }

  async removeDns(hostname: string): Promise<void> {
    const config = this.load();
    if (!config) return;

    try {
      const client = this.getClient(config.token);
      const zoneId = await this.resolveZoneId(client, config.domain);
      const existing = await client.dns.records.list({
        zone_id: zoneId,
        type: "CNAME",
        name: { exact: hostname },
      });
      const records = existing.getPaginatedItems();
      for (const record of records) {
        if (record?.id) {
          await client.dns.records.delete(record.id, { zone_id: zoneId });
        }
      }
    } catch (err) {
      consola.debug(`DNS cleanup failed for ${hostname}: ${toError(err).message}`);
    }
  }

  start(): void {
    if (this.pidFile.read() !== null) {
      this.stop();
    }

    if (!this.isInstalled()) {
      throw cloudflaredNotFoundError();
    }

    const config = this.load();
    if (!config?.tunnelToken) {
      throw cloudflareConfigNotFoundError();
    }

    mkdirSecure(this.cfDir);
    const logFd = openSync(this.logPath, "a", 0o600);

    try {
      const child = spawn(
        this.cloudflaredBin,
        ["tunnel", "--no-autoupdate", "run", "--token", config.tunnelToken],
        {
          detached: true,
          stdio: ["ignore", logFd, logFd],
        },
      );
      child.unref();
      this.pidFile.write(child.pid!);
    } finally {
      try {
        closeSync(logFd);
      } catch (err) {
        consola.debug(`Failed to close log fd: ${toError(err).message}`);
      }
    }

    sleepSync(1000);
    if (this.pidFile.read() === null) {
      let logTail = "";
      try {
        const lines = readFileSync(this.logPath, "utf-8").trim().split("\n");
        logTail = lines.slice(-8).join("\n");
      } catch (err) {
        consola.debug(`Could not read cloudflared log: ${toError(err).message}`);
      }
      throw cloudflaredStartFailedError(logTail || undefined);
    }
  }

  stop(): void {
    this.pidFile.kill("SIGTERM");
  }

  ensureRunning(): void {
    if (this.pidFile.read() !== null) return;
    this.start();
  }

  private async deleteExistingTunnel(
    client: Cloudflare,
    accountId: string,
    name: string,
  ): Promise<void> {
    const page = await client.zeroTrust.tunnels.cloudflared.list({
      account_id: accountId,
      name,
      is_deleted: false,
    });
    const tunnels = page.getPaginatedItems();
    for (const t of tunnels) {
      if (!t.id) continue;
      try {
        await client.zeroTrust.tunnels.cloudflared.connections.delete(t.id, {
          account_id: accountId,
        });
      } catch (err) {
        consola.debug(`Failed to clean connections for tunnel ${t.id}: ${toError(err).message}`);
      }
      try {
        await client.zeroTrust.tunnels.cloudflared.delete(t.id, {
          account_id: accountId,
        });
        consola.debug(`Deleted stale tunnel ${t.id} (name: ${name})`);
      } catch (err) {
        consola.debug(`Failed to delete stale tunnel ${t.id}: ${toError(err).message}`);
      }
    }
  }

  private loadRoutes(): TunnelRoute[] {
    if (!existsSync(this.routesPath)) return [];
    try {
      return JSON.parse(readFileSync(this.routesPath, "utf-8")) as TunnelRoute[];
    } catch (err) {
      consola.warn(`Routes file corrupt: ${toError(err).message}`);
      return [];
    }
  }

  private saveRoutes(routes: TunnelRoute[]): void {
    writeSecure(this.routesPath, JSON.stringify(routes, null, 2));
  }

  private buildIngress(routes: TunnelRoute[]): Array<{ hostname?: string; service: string }> {
    const ingress: Array<{ hostname?: string; service: string }> = routes.map((route) => ({
      hostname: route.hostname,
      service: route.service,
    }));
    ingress.push({ service: "http_status:404" });
    return ingress;
  }

  private async resolveAccountId(client: Cloudflare, domain?: string): Promise<string> {
    // Prefer domain-based zone lookup to ensure correct account with multi-account tokens
    if (domain) {
      try {
        const page = await client.zones.list({ name: domain, status: "active" });
        const zones = page.getPaginatedItems();
        if (zones.length > 0 && zones[0].account?.id) {
          return zones[0].account.id;
        }
      } catch (err) {
        consola.debug(`Zone lookup failed, falling back to account list: ${toError(err).message}`);
      }
    }

    try {
      const page = await client.accounts.list();
      const accounts = page.getPaginatedItems();
      if (accounts.length > 0) {
        return accounts[0].id;
      }
    } catch (err) {
      consola.debug(`Account list failed: ${toError(err).message}`);
    }

    throw cloudflareNoAccountsError();
  }

  private zoneIdCache = new Map<string, string>();

  private async resolveZoneId(client: Cloudflare, domain: string): Promise<string> {
    const cached = this.zoneIdCache.get(domain);
    if (cached) return cached;
    const page = await client.zones.list({ name: domain, status: "active" });
    const zones = page.getPaginatedItems();
    if (zones.length === 0) {
      throw cloudflareNoZoneError(domain);
    }
    this.zoneIdCache.set(domain, zones[0].id);
    return zones[0].id;
  }
}

export function resolveTunnelHostnames(network: {
  tunnelHostnames?: string[];
  tunnelHostname?: string | null;
}): string[] {
  if (network.tunnelHostnames?.length) return network.tunnelHostnames;
  if (network.tunnelHostname) return [network.tunnelHostname];
  return [];
}
