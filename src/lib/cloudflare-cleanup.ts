import { consola } from "consola";
import type { CloudflareService } from "../services/cloudflare.ts";
import { toError } from "./utils.ts";

export async function cleanupCloudflareResources(
  cloudflare: CloudflareService,
  vmId: string,
  hostnames: string[],
): Promise<void> {
  if (hostnames.length > 0) {
    cloudflare.removeRoute(vmId);
    try {
      await cloudflare.pushConfigWithRetry();
    } catch (err) {
      consola.debug(`Config push failed during cleanup: ${toError(err).message}`);
    }
  }

  await Promise.all(
    [...new Set(hostnames)].map(async (hostname) => {
      try {
        await cloudflare.removeDns(hostname);
      } catch (err) {
        consola.debug(`DNS cleanup failed for ${hostname}: ${toError(err).message}`);
      }
    }),
  );
}
