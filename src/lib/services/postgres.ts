import { randomBytes } from "node:crypto";
import type { AccessoryConfig, HealthCheckConfig } from "../toml/parser.ts";

export interface PostgresProvisionConfig {
  /** Database name (default: service name) */
  database: string;
  /** Username (default: "vmsan") */
  user: string;
  /** Password (auto-generated 32-char alphanum) */
  password: string;
  /** Port (default: 5432) */
  port: number;
}

export interface PostgresEnvVars {
  DATABASE_URL: string;
  PGHOST: string;
  PGPORT: string;
  PGUSER: string;
  PGPASSWORD: string;
  PGDATABASE: string;
}

export function buildPostgresConfig(
  serviceName: string,
  accessory: AccessoryConfig,
): PostgresProvisionConfig {
  return {
    database: accessory.env?.database ?? serviceName,
    user: accessory.env?.user ?? "vmsan",
    password: randomBytes(24).toString("base64url").slice(0, 32),
    port: accessory.env?.port ? Number(accessory.env.port) : 5432,
  };
}

export function getPostgresEnvVars(
  config: PostgresProvisionConfig,
  meshIp: string,
): PostgresEnvVars {
  return {
    DATABASE_URL: `postgresql://${config.user}:${config.password}@${meshIp}:${config.port}/${config.database}`,
    PGHOST: meshIp,
    PGPORT: String(config.port),
    PGUSER: config.user,
    PGPASSWORD: config.password,
    PGDATABASE: config.database,
  };
}

export function getPostgresHealthCheck(): HealthCheckConfig {
  return {
    type: "tcp",
    port: 5432,
    interval: 5,
    timeout: 3,
    retries: 5,
  };
}

export function getPostgresSetupCommands(config: PostgresProvisionConfig): string[] {
  return [
    `pg_isready -U postgres --timeout=30`,
    `sudo -u postgres psql -c "CREATE USER ${config.user} WITH PASSWORD '${config.password}';"`,
    `sudo -u postgres psql -c "CREATE DATABASE ${config.database} OWNER ${config.user};"`,
  ];
}
