import type { NetworkErrorCode } from "./codes.ts";
import { VmsanError } from "./base.ts";

export class NetworkError extends VmsanError {
  constructor(code: NetworkErrorCode, options: { message: string; fix?: string }) {
    super(code, options);
    this.name = "NetworkError";
  }
}

export const defaultInterfaceNotFoundError = (): NetworkError =>
  new NetworkError("ERR_NETWORK_DEFAULT_INTERFACE", {
    message: "Could not determine default network interface. Check your network configuration.",
  });
