/**
 * Type definitions for vmsan.toml declarative configuration.
 */

export interface VmsanTomlService {
  runtime: string;
  build?: string;
  start: string;
  env?: Record<string, string>;
  depends_on?: string[];
  connect_to?: string[];
}

export interface VmsanTomlAccessory {
  type: string;
  version?: string;
}

export interface VmsanTomlTunnel {
  hostname: string;
}

export interface VmsanToml {
  project?: string;
  services?: Record<string, VmsanTomlService>;
  accessories?: Record<string, VmsanTomlAccessory>;
  tunnel?: VmsanTomlTunnel;
}
