import { randomBytes } from "node:crypto";
import type { AccessoryConfig, HealthCheckConfig } from "../toml/parser.ts";

export interface RedisProvisionConfig {
  /** Auth password (generated if auth enabled) */
  password: string | null;
  /** Port (default: 6379) */
  port: number;
  /** Whether auth is enabled */
  authEnabled: boolean;
}

export interface RedisEnvVars {
  REDIS_URL: string;
  REDIS_HOST: string;
  REDIS_PORT: string;
  REDIS_PASSWORD?: string;
}

export function buildRedisConfig(
  _serviceName: string,
  accessory: AccessoryConfig,
): RedisProvisionConfig {
  const authEnabled = accessory.env?.auth === "true";
  return {
    password: authEnabled ? randomBytes(24).toString("base64url").slice(0, 32) : null,
    port: accessory.env?.port ? Number(accessory.env.port) : 6379,
    authEnabled,
  };
}

export function getRedisEnvVars(config: RedisProvisionConfig, meshIp: string): RedisEnvVars {
  const vars: RedisEnvVars = {
    REDIS_URL:
      config.authEnabled && config.password
        ? `redis://:${config.password}@${meshIp}:${config.port}`
        : `redis://${meshIp}:${config.port}`,
    REDIS_HOST: meshIp,
    REDIS_PORT: String(config.port),
  };

  if (config.authEnabled && config.password) {
    vars.REDIS_PASSWORD = config.password;
  }

  return vars;
}

export function getRedisHealthCheck(): HealthCheckConfig {
  return {
    type: "tcp",
    port: 6379,
    interval: 5,
    timeout: 3,
    retries: 5,
  };
}

export function getRedisSetupCommands(config: RedisProvisionConfig): string[] {
  if (!config.authEnabled || !config.password) {
    return [];
  }

  return [`redis-cli CONFIG SET requirepass "${config.password}"`];
}
