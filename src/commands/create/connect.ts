import { agentTimeoutError } from "../../errors/index.ts";

export async function waitForAgent(
  guestIp: string,
  port: number,
  timeoutMs = 60_000,
): Promise<void> {
  const start = Date.now();
  const url = `http://${guestIp}:${port}/health`;
  while (Date.now() - start < timeoutMs) {
    try {
      const res = await fetch(url, { signal: AbortSignal.timeout(2000) });
      if (res.ok) return;
    } catch {
      // Agent not ready yet
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw agentTimeoutError(guestIp, timeoutMs);
}
