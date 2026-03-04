import type { VmsanContext } from "../context.ts";
import { definePlugin, type VmsanPlugin } from "../plugin.ts";
import type { VmState } from "../lib/vm-state.ts";
import { CloudflareService, resolveTunnelHostnames } from "../services/cloudflare.ts";
import { cloudflaredNotFoundError } from "../errors/index.ts";
import { cleanupCloudflareResources } from "../lib/cloudflare-cleanup.ts";
import { toError } from "../lib/utils.ts";

interface SetupTunnelResult {
  hostnames: string[];
  primaryHostname: string;
}

async function setupTunnelRoutes(
  cloudflare: CloudflareService,
  state: VmState,
  hostnames: string[],
  ports: number[],
): Promise<SetupTunnelResult> {
  const { tunnelId } = await cloudflare.createTunnel();
  const dnsPromises: Promise<void>[] = [];
  for (let i = 0; i < hostnames.length; i++) {
    const hostname = hostnames[i];
    const port = ports[i] ?? ports[0];
    cloudflare.addRoute({
      vmId: state.id,
      hostname,
      service: `http://${state.network.guestIp}:${port}`,
    });
    dnsPromises.push(cloudflare.addDns(hostname, tunnelId));
  }
  await Promise.all(dnsPromises);
  await cloudflare.pushConfig();
  cloudflare.reload();
  return { hostnames, primaryHostname: hostnames[0] };
}

export function cloudflarePlugin(baseDir: string): VmsanPlugin {
  return definePlugin({
    name: "cloudflare-tunnel",
    setup(ctx) {
      const cloudflare = new CloudflareService(baseDir);

      // Pre-flight: abort if cloudflared missing + ports requested
      ctx.hooks.hook("vm:beforeCreate", ({ options }) => {
        const ports = (options as Record<string, unknown>).ports as number[] | undefined;
        if (ports?.length && cloudflare.isConfigured() && !cloudflare.isInstalled()) {
          throw cloudflaredNotFoundError();
        }
      });

      // Create tunnels after VM is running
      ctx.hooks.hook("vm:afterCreate", async (state) => {
        if (!state.network.publishedPorts?.length || !cloudflare.isConfigured()) return;
        const cfConfig = cloudflare.load()!;
        const baseHostname = `${state.id}.${cfConfig.domain}`;
        const hostnames = state.network.publishedPorts.map((port) =>
          state.network.publishedPorts.length === 1
            ? baseHostname
            : `${state.id}-${port}.${cfConfig.domain}`,
        );
        await withTunnelSetup(ctx, cloudflare, state, hostnames, state.network.publishedPorts, {
          startMsg: "Setting up Cloudflare Tunnel...",
          successMsg: (h) => `Cloudflare Tunnel: https://${h}`,
          failMsg: (e) =>
            `Cloudflare Tunnel setup failed: ${e}. Port forwarding via DNAT still active.`,
          onSuccess: (result) => {
            ctx.store.update(state.id, {
              network: {
                ...state.network,
                tunnelHostname: baseHostname,
                tunnelHostnames: result.hostnames,
              },
            });
          },
        });
      });

      // Restore tunnels on start
      ctx.hooks.hook("vm:afterStart", async (state) => {
        const savedHostnames = resolveTunnelHostnames(state.network);
        if (!savedHostnames.length || !cloudflare.isConfigured()) return;
        await withTunnelSetup(
          ctx,
          cloudflare,
          state,
          savedHostnames,
          state.network.publishedPorts,
          {
            startMsg: "Restoring Cloudflare Tunnel...",
            successMsg: (h) => `Cloudflare Tunnel restored: https://${h}`,
            failMsg: (e) => `Cloudflare restore failed: ${e}. DNAT still active.`,
          },
        );
      });

      // Cleanup tunnels before stop
      ctx.hooks.hook("vm:beforeStop", async ({ vmId, state }) => {
        const tunnelHostnames = resolveTunnelHostnames(state.network);
        const routeHostnames = cloudflare.getHostnames(vmId);
        const allHostnames = [...new Set([...tunnelHostnames, ...routeHostnames])];
        if (!allHostnames.length) return;
        try {
          cloudflare.removeRoute(vmId);
          await cloudflare.pushConfig();
          await Promise.all(allHostnames.map((hostname) => cloudflare.removeDns(hostname)));
          cloudflare.reload();
        } catch (err) {
          ctx.logger.debug(`Cloudflare cleanup failed for VM ${vmId}: ${toError(err).message}`);
        }
      });
    },
  });
}

interface TunnelSetupMessages {
  startMsg: string;
  successMsg: (primaryHostname: string) => string;
  failMsg: (error: string) => string;
  onSuccess?: (result: SetupTunnelResult) => void;
}

async function withTunnelSetup(
  ctx: VmsanContext,
  cloudflare: CloudflareService,
  state: VmState,
  hostnames: string[],
  ports: number[],
  msgs: TunnelSetupMessages,
): Promise<void> {
  const attemptedHostnames: string[] = [];
  try {
    ctx.logger.start(msgs.startMsg);
    const result = await setupTunnelRoutes(cloudflare, state, hostnames, ports);
    attemptedHostnames.push(...result.hostnames);
    msgs.onSuccess?.(result);
    ctx.logger.success(msgs.successMsg(result.primaryHostname));
  } catch (err) {
    await cleanupCloudflareResources(cloudflare, state.id, attemptedHostnames);
    ctx.logger.warn(msgs.failMsg(toError(err).message));
  }
}
