export {
  buildPostgresConfig,
  getPostgresEnvVars,
  getPostgresHealthCheck,
  getPostgresSetupCommands,
} from "./postgres.ts";
export type { PostgresProvisionConfig, PostgresEnvVars } from "./postgres.ts";

export {
  buildRedisConfig,
  getRedisEnvVars,
  getRedisHealthCheck,
  getRedisSetupCommands,
} from "./redis.ts";
export type { RedisProvisionConfig, RedisEnvVars } from "./redis.ts";

export type SupportedServiceType = "postgres" | "redis";

export function isSupportedServiceType(type: string): type is SupportedServiceType {
  return type === "postgres" || type === "redis";
}
