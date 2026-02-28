import type { ErrorOptions } from "evlog";
import type { TimeoutErrorCode } from "./codes.ts";
import { VmsanError } from "./base.ts";

export class TimeoutError extends VmsanError {
  readonly target?: string;
  readonly timeoutMs?: number;

  constructor(
    code: TimeoutErrorCode,
    options: ErrorOptions & { target?: string; timeoutMs?: number },
  ) {
    super(code, options);
    this.name = "TimeoutError";
    this.target = options.target;
    this.timeoutMs = options.timeoutMs;
  }

  override toJSON(): Record<string, unknown> {
    return {
      ...super.toJSON(),
      ...(this.target !== undefined && { target: this.target }),
      ...(this.timeoutMs !== undefined && { timeoutMs: this.timeoutMs }),
    };
  }
}

export const socketTimeoutError = (socketPath: string): TimeoutError =>
  new TimeoutError("ERR_TIMEOUT_SOCKET", {
    target: socketPath,
    message: `Timeout waiting for API socket at ${socketPath}`,
  });

export const lockTimeoutError = (lockName: string): TimeoutError =>
  new TimeoutError("ERR_TIMEOUT_LOCK", {
    target: lockName,
    message: `Timed out waiting for ${lockName} lock`,
  });

export const agentTimeoutError = (guestIp: string, timeoutMs: number): TimeoutError =>
  new TimeoutError("ERR_TIMEOUT_AGENT", {
    target: guestIp,
    timeoutMs,
    message: `Agent at ${guestIp} not available after ${timeoutMs}ms`,
  });
