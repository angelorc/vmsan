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

export const nftSetupFailedError = (message: string): NetworkError =>
  new NetworkError("ERR_NFT_SETUP_FAILED", {
    message: `nftables setup failed: ${message}`,
    fix: "Check that vmsan-nftables is installed and nftables kernel module is loaded. Run 'vmsan doctor' for diagnostics.",
  });

export const nftTeardownFailedError = (message: string): NetworkError =>
  new NetworkError("ERR_NFT_TEARDOWN_FAILED", {
    message: `nftables teardown failed: ${message}`,
  });

export const nftBinaryMissingError = (): NetworkError =>
  new NetworkError("ERR_NFT_BINARY_MISSING", {
    message: "vmsan-nftables binary not found at ~/.vmsan/bin/vmsan-nftables",
    fix: 'Run "curl -fsSL https://vmsan.dev/install | bash" to install.',
  });

export const nftTableMissingError = (vmId: string): NetworkError =>
  new NetworkError("ERR_NFT_TABLE_MISSING", {
    message: `nftables table vmsan_${vmId} not found`,
  });
