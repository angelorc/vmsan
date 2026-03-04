import { randomBytes } from "node:crypto";
import { closeSync, existsSync, openSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { spawn } from "node:child_process";
import Cloudflare from "cloudflare";
import pRetry, { AbortError } from "p-retry";
import { mkdirSecure, writeSecure } from "../lib/utils.ts";
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
}

export interface TunnelRoute {
  vmId: string;
  hostname: string;
  service: string;
}

const WAIT_ARRAY = new Int32Array(new SharedArrayBuffer(4));

export class CloudflareService {
  private readonly cfDir: string;
  private readonly configPath: string;
  private readonly routesPath: string;
  private readonly tunnelConfigPath: string;
  private readonly credentialsPath: string;
  private readonly logPath: string;
  private readonly cloudflaredBin: string;
  private readonly lock: FileLock;
  private readonly pidFile: PidFile;

  constructor(baseDir: string) {
    this.cfDir = join(baseDir, "cloudflare");
    this.configPath = join(this.cfDir, "cloudflare.json");
    this.routesPath = join(this.cfDir, "routes.json");
    this.tunnelConfigPath = join(this.cfDir, "config.yml");
    this.credentialsPath = join(this.cfDir, "tunnel-credentials.json");
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
    } catch {
      // Config file missing or corrupt — treat as unconfigured
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

      const hasCredentials =
        typeof config.tunnelId === "string" ? this.hasCredentials(config.tunnelId) : false;
      const previousTunnelId = config.tunnelId;

      if (config.tunnelId && hasCredentials) {
        return { tunnelId: config.tunnelId };
      }

      const client = this.getClient(config.token);
      const accountId = config.accountId || (await this.resolveAccountId(client, config.domain));
      const tunnelSecret = randomBytes(32).toString("base64");
      const tunnelName =
        config.tunnelId && !hasCredentials ? `vmsan-${Date.now().toString(36)}` : "vmsan";

      const tunnel = await client.zeroTrust.tunnels.cloudflared.create({
        account_id: accountId,
        name: tunnelName,
        tunnel_secret: tunnelSecret,
        config_src: "local",
      });

      const tunnelId = tunnel.id;
      if (!tunnelId) throw cloudflareTunnelNoIdError();

      mkdirSecure(this.cfDir);
      writeSecure(
        this.credentialsPath,
        JSON.stringify({
          AccountTag: accountId,
          TunnelSecret: tunnelSecret,
          TunnelID: tunnelId,
        }),
      );

      config.tunnelId = tunnelId;
      config.accountId = accountId;
      this.save(config);

      if (previousTunnelId && previousTunnelId !== tunnelId) {
        try {
          await client.zeroTrust.tunnels.cloudflared.delete(previousTunnelId, {
            account_id: accountId,
          });
        } catch {
          // Previous tunnel may already be deleted or inaccessible
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
    this.lock.run(() => {
      const routes = this.loadRoutes();
      const filtered = routes.filter((r) => r.vmId !== route.vmId || r.hostname !== route.hostname);
      filtered.push(route);
      this.saveRoutes(filtered);

      const config = this.load();
      if (config?.tunnelId) {
        writeSecure(this.tunnelConfigPath, this.buildConfigYml(filtered, config.tunnelId));
      }
    });
  }

  removeRoute(vmId: string): void {
    this.lock.run(() => {
      const routes = this.loadRoutes();
      const filtered = routes.filter((r) => r.vmId !== vmId);
      this.saveRoutes(filtered);

      const config = this.load();
      if (config?.tunnelId) {
        writeSecure(this.tunnelConfigPath, this.buildConfigYml(filtered, config.tunnelId));
      }
    });
  }

  async addDns(hostname: string, tunnelId: string): Promise<void> {
    const config = this.load();
    if (!config) throw cloudflareNotConfiguredError();

    const client = this.getClient(config.token);
    const zoneId = await this.resolveZoneId(client, config.domain);

    const existing = await client.dns.records.list({
      zone_id: zoneId,
      type: "CNAME",
      name: { exact: hostname },
    });
    const records = existing.getPaginatedItems();
    if (records.length > 0) {
      await client.dns.records.update(records[0].id, {
        zone_id: zoneId,
        type: "CNAME",
        name: hostname,
        content: `${tunnelId}.cfargotunnel.com`,
        proxied: true,
        ttl: 1,
      });
      return;
    }

    await client.dns.records.create({
      zone_id: zoneId,
      type: "CNAME",
      name: hostname,
      content: `${tunnelId}.cfargotunnel.com`,
      proxied: true,
      ttl: 1,
    });
  }

  async removeDns(hostname: string): Promise<void> {
    const config = this.load();
    if (!config) return;

    try {
      const client = this.getClient(config.token);
      const zoneId = await this.resolveZoneId(client, config.domain);
      for (let attempt = 0; attempt < 5; attempt++) {
        const existing = await client.dns.records.list({
          zone_id: zoneId,
          type: "CNAME",
          name: { exact: hostname },
        });
        const records = existing.getPaginatedItems();
        if (records.length === 0) {
          if (attempt < 4) {
            Atomics.wait(WAIT_ARRAY, 0, 0, 250);
          }
          continue;
        }
        for (const record of records) {
          if (record?.id) {
            await client.dns.records.delete(record.id, { zone_id: zoneId });
          }
        }
        Atomics.wait(WAIT_ARRAY, 0, 0, 150);
      }
    } catch {
      // Best-effort DNS cleanup — API may be unreachable during teardown
    }
  }

  start(): void {
    if (this.pidFile.read() !== null) {
      this.stop();
    }

    if (!this.isInstalled()) {
      throw cloudflaredNotFoundError();
    }

    if (!existsSync(this.tunnelConfigPath)) {
      throw cloudflareConfigNotFoundError();
    }

    mkdirSecure(this.cfDir);
    const logFd = openSync(this.logPath, "a", 0o600);

    try {
      const child = spawn(
        this.cloudflaredBin,
        ["tunnel", "--config", this.tunnelConfigPath, "run"],
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
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== "EBADF") throw error;
      }
    }

    Atomics.wait(WAIT_ARRAY, 0, 0, 1000);
    if (this.pidFile.read() === null) {
      let logTail = "";
      try {
        const lines = readFileSync(this.logPath, "utf-8").trim().split("\n");
        logTail = lines.slice(-8).join("\n");
      } catch {
        // Log file may not exist yet if cloudflared crashed immediately
      }
      throw cloudflaredStartFailedError(logTail || undefined);
    }
  }

  stop(): void {
    this.pidFile.kill("SIGTERM");
  }

  reload(): void {
    this.stop();
    this.start();
  }

  ensureRunning(): void {
    if (this.pidFile.read() !== null) return;
    this.start();
  }

  private loadRoutes(): TunnelRoute[] {
    if (!existsSync(this.routesPath)) return [];
    try {
      return JSON.parse(readFileSync(this.routesPath, "utf-8")) as TunnelRoute[];
    } catch {
      // Routes file missing or corrupt — treat as empty
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

  private buildConfigYml(routes: TunnelRoute[], tunnelId: string): string {
    const lines = [`tunnel: ${tunnelId}`, `credentials-file: ${this.credentialsPath}`, `ingress:`];
    for (const route of routes) {
      lines.push(`  - hostname: ${route.hostname}`);
      lines.push(`    service: ${route.service}`);
    }
    lines.push(`  - service: http_status:404`);
    return lines.join("\n");
  }

  private hasCredentials(expectedTunnelId?: string): boolean {
    if (!existsSync(this.credentialsPath)) return false;
    try {
      const creds = JSON.parse(readFileSync(this.credentialsPath, "utf-8")) as {
        AccountTag?: string;
        TunnelSecret?: string;
        TunnelID?: string;
      };
      if (!creds.AccountTag || !creds.TunnelSecret || !creds.TunnelID) {
        return false;
      }
      if (expectedTunnelId && creds.TunnelID !== expectedTunnelId) {
        return false;
      }
      return true;
    } catch {
      // Credentials file missing or corrupt — treat as absent
      return false;
    }
  }

  private async resolveAccountId(client: Cloudflare, domain?: string): Promise<string> {
    try {
      const page = await client.accounts.list();
      const accounts = page.getPaginatedItems();
      if (accounts.length > 0) {
        return accounts[0].id;
      }
    } catch {
      // Token may lack account-level permissions — fall through to zone lookup
    }

    if (domain) {
      const page = await client.zones.list({ name: domain, status: "active" });
      const zones = page.getPaginatedItems();
      if (zones.length > 0 && zones[0].account?.id) {
        return zones[0].account.id;
      }
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
