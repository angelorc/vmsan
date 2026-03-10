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
): FirecrackerApiError => {
  const opts: ErrorOptions & { method: string; path: string; httpStatus: number } = {
    method,
    path,
    httpStatus,
    message: `${method} ${path} failed (${httpStatus}): ${body}`,
  };

  if (body.includes("/dev/net/tun") && body.includes("Permission denied")) {
    opts.why =
      "Firecracker cannot open /dev/net/tun inside the jailer chroot. " +
      "This usually means the filesystem where ~/.vmsan resides is mounted with the 'nodev' option.";
    opts.fix = [
      "Check mount options:  findmnt -T ~/.vmsan -o TARGET,OPTIONS",
      "",
      "If 'nodev' appears, remount without it:",
      "  sudo mount -o remount,dev <mountpoint>",
      "",
      "Or move vmsan to a different filesystem:",
      "  export VMSAN_DIR=/var/lib/vmsan",
      "  curl -fsSL https://vmsan.dev/install | bash",
      "",
      "Run 'vmsan doctor' for a full diagnostic.",
    ].join("\n");
  }

  return new FirecrackerApiError("ERR_FIRECRACKER_API", opts);
};
