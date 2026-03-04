import type { ErrorOptions } from "evlog";
import type { CloudflareErrorCode } from "./codes.ts";
import { VmsanError } from "./base.ts";

export class CloudflareError extends VmsanError {
  readonly domain?: string;

  constructor(code: CloudflareErrorCode, options: ErrorOptions & { domain?: string }) {
    super(code, options);
    this.name = "CloudflareError";
    this.domain = options.domain;
  }

  override toJSON(): Record<string, unknown> {
    return { ...super.toJSON(), ...(this.domain !== undefined && { domain: this.domain }) };
  }
}

export const cloudflareNotConfiguredError = (): CloudflareError =>
  new CloudflareError("ERR_CLOUDFLARE_NOT_CONFIGURED", {
    message: "Cloudflare not configured",
  });

export const cloudflareTunnelNoIdError = (): CloudflareError =>
  new CloudflareError("ERR_CLOUDFLARE_TUNNEL_NO_ID", {
    message: "Cloudflare API returned tunnel without id",
  });

export const cloudflaredNotFoundError = (): CloudflareError =>
  new CloudflareError("ERR_CLOUDFLARE_BINARY_NOT_FOUND", {
    message: "cloudflared binary not found. Ensure it is installed in the vmsan bin directory.",
    fix: "Place the cloudflared binary in the vmsan bin directory.",
  });

export const cloudflareConfigNotFoundError = (): CloudflareError =>
  new CloudflareError("ERR_CLOUDFLARE_CONFIG_NOT_FOUND", {
    message: "Cloudflare tunnel config not found",
  });

export const cloudflaredStartFailedError = (logTail?: string): CloudflareError =>
  new CloudflareError("ERR_CLOUDFLARE_START_FAILED", {
    message: `cloudflared failed to start${logTail ? `. Recent logs:\n${logTail}` : ""}`,
  });

export const cloudflareNoAccountsError = (): CloudflareError =>
  new CloudflareError("ERR_CLOUDFLARE_NO_ACCOUNTS", {
    message: "No Cloudflare accounts found for this token",
  });

export const cloudflareNoZoneError = (domain: string): CloudflareError =>
  new CloudflareError("ERR_CLOUDFLARE_NO_ZONE", {
    domain,
    message: `No active Cloudflare zone found for domain: ${domain}`,
  });

export const cloudflareTokenInvalidError = (): CloudflareError =>
  new CloudflareError("ERR_CLOUDFLARE_TOKEN_INVALID", {
    message:
      "Provided --cloudflare-token looks like a tunnel run token (from 'cloudflared service install'). " +
      "Use a Cloudflare API token with Zone:DNS Edit and Cloudflare Tunnel Edit permissions.",
  });

export const cloudflareTokenInactiveError = (status: string): CloudflareError =>
  new CloudflareError("ERR_CLOUDFLARE_TOKEN_INACTIVE", {
    message: `Cloudflare token is not active (status: ${status})`,
  });

export const cloudflareNoZoneForDomainError = (domain: string): CloudflareError =>
  new CloudflareError("ERR_CLOUDFLARE_NO_ZONE", {
    domain,
    message: `No active Cloudflare zone found for --cloudflare-domain '${domain}'`,
  });
