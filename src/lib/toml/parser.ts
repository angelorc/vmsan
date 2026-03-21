import { parse } from "smol-toml";
import { readFileSync } from "node:fs";
import { findClosestMatch } from "./suggest.ts";

// ---------- Schema types ----------

export interface VmsanToml {
  // Single-service format (flat)
  runtime?: string;
  build?: string;
  start?: string;

  // Multi-service format
  services?: Record<string, ServiceConfig>;

  // Accessories (databases, caches)
  accessories?: Record<string, AccessoryConfig>;

  // Deploy settings
  deploy?: DeployConfig;

  // Tunnel settings
  tunnel?: TunnelConfig;
}

export interface ServiceConfig {
  runtime?: string;
  build?: string;
  start?: string;
  env?: Record<string, string>;
  depends_on?: string[];
  connect_to?: string[];
  service?: string;
  publish_ports?: string[];
  memory?: number;
  vcpus?: number;
  disk?: string;
  network_policy?: string;
  allowed_domains?: string[];
  health_check?: HealthCheckConfig;
}

export interface AccessoryConfig {
  type: string; // "postgres", "redis", etc.
  version?: string;
  env?: Record<string, string>;
}

export interface DeployConfig {
  release?: string; // Command to run before start (e.g., "npx prisma migrate deploy")
}

export interface TunnelConfig {
  hostname?: string;
  hostnames?: string[];
}

export interface HealthCheckConfig {
  type: "http" | "tcp" | "exec";
  path?: string; // for HTTP
  port?: number; // for HTTP/TCP
  command?: string; // for exec
  interval?: number; // seconds
  timeout?: number; // seconds
  retries?: number;
}

// ---------- Known fields ----------

const TOP_LEVEL_KEYS = new Set([
  "runtime",
  "build",
  "start",
  "services",
  "accessories",
  "deploy",
  "tunnel",
]);

const SERVICE_KEYS = new Set([
  "runtime",
  "build",
  "start",
  "env",
  "depends_on",
  "connect_to",
  "service",
  "publish_ports",
  "memory",
  "vcpus",
  "disk",
  "network_policy",
  "allowed_domains",
  "health_check",
]);

const ACCESSORY_KEYS = new Set(["type", "version", "env"]);

const DEPLOY_KEYS = new Set(["release"]);

const TUNNEL_KEYS = new Set(["hostname", "hostnames"]);

const HEALTH_CHECK_KEYS = new Set([
  "type",
  "path",
  "port",
  "command",
  "interval",
  "timeout",
  "retries",
]);

// ---------- Validation helpers ----------

function unknownFieldError(field: string, section: string, known: Set<string>): Error {
  const suggestion = findClosestMatch(field, known);
  const hint = suggestion ? ` Did you mean "${suggestion}"?` : "";
  const validFields = [...known].sort().join(", ");
  return new Error(`Unknown field "${field}" in ${section}. Valid fields: ${validFields}.${hint}`);
}

function validateKeys(obj: Record<string, unknown>, known: Set<string>, section: string): void {
  for (const key of Object.keys(obj)) {
    if (!known.has(key)) {
      throw unknownFieldError(key, section, known);
    }
  }
}

function validateHealthCheck(hc: Record<string, unknown>, section: string): void {
  validateKeys(hc, HEALTH_CHECK_KEYS, `${section}.health_check`);
  if (!hc.type) {
    throw new Error(`Missing required field "type" in ${section}.health_check`);
  }
  const validTypes = ["http", "tcp", "exec"];
  if (!validTypes.includes(hc.type as string)) {
    throw new Error(
      `Invalid health_check type "${hc.type}" in ${section}. Valid types: ${validTypes.join(", ")}`,
    );
  }
}

// ---------- Public API ----------

export function parseVmsanToml(content: string): VmsanToml {
  let raw: Record<string, unknown>;
  try {
    raw = parse(content) as Record<string, unknown>;
  } catch (error) {
    if (error instanceof Error) {
      throw new Error(`Invalid TOML: ${error.message}`);
    }
    throw new Error(`Invalid TOML: ${String(error)}`);
  }

  // Validate top-level keys
  validateKeys(raw, TOP_LEVEL_KEYS, "vmsan.toml");

  // Validate services
  if (raw.services && typeof raw.services === "object") {
    const services = raw.services as Record<string, Record<string, unknown>>;
    for (const [name, service] of Object.entries(services)) {
      validateKeys(service, SERVICE_KEYS, `services.${name}`);
      if (service.health_check && typeof service.health_check === "object") {
        validateHealthCheck(service.health_check as Record<string, unknown>, `services.${name}`);
      }
    }
  }

  // Validate accessories
  if (raw.accessories && typeof raw.accessories === "object") {
    const accessories = raw.accessories as Record<string, Record<string, unknown>>;
    for (const [name, accessory] of Object.entries(accessories)) {
      validateKeys(accessory, ACCESSORY_KEYS, `accessories.${name}`);
      if (!accessory.type) {
        throw new Error(`Missing required field "type" in accessories.${name}`);
      }
    }
  }

  // Validate deploy
  if (raw.deploy && typeof raw.deploy === "object") {
    validateKeys(raw.deploy as Record<string, unknown>, DEPLOY_KEYS, "deploy");
  }

  // Validate tunnel
  if (raw.tunnel && typeof raw.tunnel === "object") {
    validateKeys(raw.tunnel as Record<string, unknown>, TUNNEL_KEYS, "tunnel");
  }

  return raw as unknown as VmsanToml;
}

export function loadVmsanToml(filePath: string): VmsanToml {
  const content = readFileSync(filePath, "utf-8");
  return parseVmsanToml(content);
}

export function isMultiService(config: VmsanToml): boolean {
  return config.services !== undefined && Object.keys(config.services).length > 0;
}

export function normalizeToml(config: VmsanToml): Record<string, ServiceConfig> {
  if (isMultiService(config)) {
    return { ...config.services! };
  }

  // Convert flat single-service to multi-service with "app" as default name
  const service: ServiceConfig = {};
  if (config.runtime) service.runtime = config.runtime;
  if (config.build) service.build = config.build;
  if (config.start) service.start = config.start;

  return { app: service };
}
