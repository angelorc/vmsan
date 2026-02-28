import { EvlogError, type ErrorOptions } from "evlog";
import type { VmsanErrorCode } from "./codes.ts";

export class VmsanError extends EvlogError {
  readonly code: VmsanErrorCode;

  constructor(code: VmsanErrorCode, options: ErrorOptions) {
    super(options);
    this.name = "VmsanError";
    this.code = code;
    if (Error.captureStackTrace) {
      Error.captureStackTrace(this, new.target);
    }
  }

  override toJSON(): Record<string, unknown> {
    return { ...super.toJSON(), code: this.code };
  }
}
