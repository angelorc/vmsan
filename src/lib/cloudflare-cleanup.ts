import type { CloudflareService } from "../services/cloudflare.ts";

export async function cleanupCloudflareResources(
  cloudflare: CloudflareService,
  vmId: string,
  hostnames: string[],
): Promise<void> {
  if (hostnames.length > 0) {
    cloudflare.removeRoute(vmId);
    try {
      await cloudflare.pushConfigWithRetry();
    } catch {
      // API push failed after retries; local config still applied via reload below
    }
    try {
      cloudflare.ensureRunning();
    } catch {
      // cloudflared process may not be running
    }
  }

  for (const hostname of new Set(hostnames)) {
    try {
      await cloudflare.removeDns(hostname);
    } catch {
      // DNS record may not exist or API unreachable during teardown
    }
  }
}
