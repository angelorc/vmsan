/**
 * Type definitions for vmsan.toml declarative configuration.
 * Re-exports canonical types from parser.ts for backward compatibility.
 */
export type {
  VmsanToml,
  ServiceConfig,
  AccessoryConfig,
  DeployConfig,
  TunnelConfig,
  HealthCheckConfig,
} from "./parser.ts";
