import type { AgentClient } from "../../services/agent.ts";
import type { Command } from "../command.ts";
import consola from "consola";
import { toError } from "../utils.ts";

const DEFAULT_BUILD_TIMEOUT_MS = 10 * 60 * 1000; // 10 minutes

export interface BuildOptions {
  /** Build command (e.g., "npm run build") */
  buildCommand: string;
  /** Agent client for the target VM */
  agent: AgentClient;
  /** Working directory in VM (default: /app) */
  cwd?: string;
  /** Environment variables */
  env?: Record<string, string>;
  /** Timeout in milliseconds (default: 10 minutes) */
  timeoutMs?: number;
  /** Stream stdout in real-time */
  onStdout?: (line: string) => void;
  /** Stream stderr in real-time */
  onStderr?: (line: string) => void;
}

export interface BuildResult {
  success: boolean;
  exitCode: number;
  output: string;
  durationMs: number;
}

export interface StartOptions {
  /** Start command (e.g., "npm start") */
  startCommand: string;
  /** Agent client */
  agent: AgentClient;
  /** Working directory */
  cwd?: string;
  /** Environment variables */
  env?: Record<string, string>;
}

/**
 * Run a build command inside a VM and wait for completion.
 */
export async function executeBuild(opts: BuildOptions): Promise<BuildResult> {
  const {
    buildCommand,
    agent,
    cwd = "/app",
    env,
    timeoutMs = DEFAULT_BUILD_TIMEOUT_MS,
    onStdout,
    onStderr,
  } = opts;

  const startTime = Date.now();
  consola.start(`Running build command: ${buildCommand}`);

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const cmd = await agent.exec(
      { cmd: "sh", args: ["-c", buildCommand], cwd, env, timeoutMs },
      { signal: controller.signal, onStdout, onStderr },
    );

    const result = await cmd.wait({ signal: controller.signal });
    const durationMs = Date.now() - startTime;

    if (result.timedOut) {
      consola.error(`Build timed out after ${Math.round(durationMs / 1000)}s`);
      return { success: false, exitCode: result.exitCode, output: result.output, durationMs };
    }

    if (!result.ok) {
      consola.error(`Build failed with exit code ${result.exitCode}`);
      return { success: false, exitCode: result.exitCode, output: result.output, durationMs };
    }

    consola.success(`Build completed in ${Math.round(durationMs / 1000)}s`);
    return { success: true, exitCode: 0, output: result.output, durationMs };
  } catch (err) {
    const durationMs = Date.now() - startTime;
    consola.error(`Build error: ${toError(err).message}`);
    return { success: false, exitCode: 1, output: toError(err).message, durationMs };
  } finally {
    clearTimeout(timeout);
  }
}

/**
 * Start an application command in the VM as a detached process.
 * Returns the Command handle for monitoring.
 */
export async function startApp(opts: StartOptions): Promise<Command> {
  const { startCommand, agent, cwd = "/app", env } = opts;

  consola.start(`Starting app: ${startCommand}`);

  try {
    const cmd = await agent.runCommand({
      cmd: "sh",
      args: ["-c", startCommand],
      cwd,
      env,
      detached: true,
    });

    consola.success(`App started (command id: ${cmd.cmdId})`);
    return cmd;
  } catch (err) {
    consola.error(`Failed to start app: ${toError(err).message}`);
    throw err;
  }
}
