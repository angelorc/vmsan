import type { ErrorOptions } from "evlog";
import type { ValidationErrorCode } from "./codes.ts";
import { VmsanError } from "./base.ts";

export class ValidationError extends VmsanError {
  readonly flag?: string;

  constructor(code: ValidationErrorCode, options: ErrorOptions & { flag?: string }) {
    super(code, options);
    this.name = "ValidationError";
    this.flag = options.flag;
  }

  override toJSON(): Record<string, unknown> {
    return { ...super.toJSON(), ...(this.flag !== undefined && { flag: this.flag }) };
  }
}

export const invalidIntegerFlagError = (
  flag: string,
  value: string,
  min: number,
  max: number,
  unitSuffix: string = "",
): ValidationError =>
  new ValidationError("ERR_VALIDATION_INTEGER", {
    flag,
    message: `Invalid --${flag}: "${value}". Must be an integer between ${min} and ${max}${unitSuffix}.`,
  });

export const invalidRuntimeError = (
  runtime: string,
  validRuntimes: readonly string[],
): ValidationError =>
  new ValidationError("ERR_VALIDATION_RUNTIME", {
    flag: "runtime",
    message: `Invalid --runtime: "${runtime}". Must be one of: ${validRuntimes.join(", ")}`,
  });

export const invalidNetworkPolicyError = (
  policy: string,
  validPolicies: readonly string[],
): ValidationError =>
  new ValidationError("ERR_VALIDATION_NETWORK_POLICY", {
    flag: "network-policy",
    message: `Invalid --network-policy: "${policy}". Must be one of: ${validPolicies.join(", ")}`,
  });

export const invalidPortError = (port: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_PORT", {
    flag: "publish-port",
    message: `Invalid port: ${port}`,
  });

export const portConflictError = (conflictSummary: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_PORT_CONFLICT", {
    flag: "publish-port",
    message: `Published port conflict: ${conflictSummary}`,
  });

export const invalidDomainError = (domain: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_DOMAIN", {
    flag: "allowed-domain",
    message: `Invalid domain: "${domain}"`,
  });

export const invalidDomainPatternError = (domain: string, detail?: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_DOMAIN", {
    flag: "allowed-domain",
    message: detail
      ? `Invalid domain pattern: "${domain}". ${detail}`
      : `Invalid domain pattern: "${domain}"`,
  });

export const invalidCidrFormatError = (cidr: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_CIDR", {
    message: `Invalid CIDR format: "${cidr}". Expected format: x.x.x.x/y`,
  });

export const invalidCidrPrefixError = (cidr: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_CIDR", {
    message: `Invalid CIDR prefix length: "${cidr}". Must be 0-32.`,
  });

export const invalidCidrOctetError = (cidr: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_CIDR", {
    message: `Invalid CIDR IP octet: "${cidr}". Each octet must be 0-255.`,
  });

export const invalidImageRefEmptyError = (): ValidationError =>
  new ValidationError("ERR_VALIDATION_IMAGE_REF", {
    flag: "from-image",
    message: "Invalid --from-image: image reference cannot be empty.",
  });

export const invalidImageRefTagError = (ref: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_IMAGE_REF", {
    flag: "from-image",
    message: `Invalid --from-image: "${ref}". Tag cannot be empty.`,
  });

export const invalidDiskSizeFormatError = (value: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_DISK_SIZE", {
    flag: "disk",
    message: `Invalid --disk: "${value}". Expected format like 10gb.`,
  });

export const invalidDiskSizeRangeError = (value: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_DISK_SIZE", {
    flag: "disk",
    message: `Invalid --disk: "${value}". Must be an integer between 1gb and 1024gb.`,
  });

export const invalidDurationError = (input: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_DURATION", {
    message: `Invalid duration: "${input}". Use format like "1h", "30m", "2h30m", or plain minutes.`,
  });

export const mutuallyExclusiveFlagsError = (flagA: string, flagB: string): ValidationError =>
  new ValidationError("ERR_VALIDATION_FLAGS", {
    message: `Cannot use ${flagA} and ${flagB} together`,
    why: "They are mutually exclusive.",
    fix: `Use either ${flagA} or ${flagB}, not both.`,
  });

export const policyConflictError = (): ValidationError =>
  new ValidationError("ERR_VALIDATION_POLICY_CONFLICT", {
    flag: "network-policy",
    message:
      "Cannot combine --network-policy deny-all with --allowed-domain, --allowed-cidr, or --denied-cidr.",
  });
