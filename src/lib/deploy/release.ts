import type { AgentClient } from "../../services/agent.ts";
import { toError } from "../utils.ts";
import consola from "consola";

const DEFAULT_TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes

export interface ReleaseOptions {
  /** The release command to execute */
  command: string;
  /** Agent client for the target VM */
  agent: AgentClient;
  /** Timeout in milliseconds (default: 5 minutes) */
  timeoutMs?: number;
  /** Callback for stdout lines */
  onStdout?: (line: string) => void;
  /** Callback for stderr lines */
  onStderr?: (line: string) => void;
}

export interface ReleaseResult {
  success: boolean;
  exitCode: number;
  output: string;
  timedOut: boolean;
  durationMs: number;
}

export async function executeRelease(opts: ReleaseOptions): Promise<ReleaseResult> {
  const { command, agent, timeoutMs = DEFAULT_TIMEOUT_MS, onStdout, onStderr } = opts;
  const startTime = Date.now();

  consola.start(`Running release command: ${command}`);

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const cmd = await agent.exec(
      { cmd: "sh", args: ["-c", command], timeoutMs },
      { signal: controller.signal, onStdout, onStderr },
    );

    const result = await cmd.wait({ signal: controller.signal });
    const durationMs = Date.now() - startTime;

    if (result.timedOut) {
      consola.error(`Release command timed out after ${Math.round(durationMs / 1000)}s`);
      return {
        success: false,
        exitCode: result.exitCode,
        output: result.output,
        timedOut: true,
        durationMs,
      };
    }

    if (!result.ok) {
      consola.error(`Release command failed with exit code ${result.exitCode}`);
      return {
        success: false,
        exitCode: result.exitCode,
        output: result.output,
        timedOut: false,
        durationMs,
      };
    }

    consola.success(`Release command completed in ${Math.round(durationMs / 1000)}s`);
    return {
      success: true,
      exitCode: 0,
      output: result.output,
      timedOut: false,
      durationMs,
    };
  } catch (err) {
    const durationMs = Date.now() - startTime;
    consola.error(`Release command error: ${toError(err).message}`);
    return {
      success: false,
      exitCode: 1,
      output: toError(err).message,
      timedOut: controller.signal.aborted,
      durationMs,
    };
  } finally {
    clearTimeout(timeout);
  }
}
