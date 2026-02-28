import type { ErrorOptions } from "evlog";
import type { FirecrackerErrorCode } from "./codes.ts";
import { VmsanError } from "./base.ts";

export class FirecrackerApiError extends VmsanError {
  readonly method: string;
  readonly path: string;
  readonly httpStatus: number;

  constructor(
    code: FirecrackerErrorCode,
    options: ErrorOptions & { method: string; path: string; httpStatus: number },
  ) {
    super(code, options);
    this.name = "FirecrackerApiError";
    this.method = options.method;
    this.path = options.path;
    this.httpStatus = options.httpStatus;
  }

  override toJSON(): Record<string, unknown> {
    return {
      ...super.toJSON(),
      method: this.method,
      path: this.path,
      httpStatus: this.httpStatus,
    };
  }
}

export const firecrackerApiError = (
  method: string,
  path: string,
  httpStatus: number,
  body: string,
): FirecrackerApiError =>
  new FirecrackerApiError("ERR_FIRECRACKER_API", {
    method,
    path,
    httpStatus,
    message: `${method} ${path} failed (${httpStatus}): ${body}`,
  });
