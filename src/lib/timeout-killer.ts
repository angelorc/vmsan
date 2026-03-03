import { spawn, type ChildProcess } from "node:child_process";

export interface SpawnTimeoutKillerOpts {
  vmId: string;
  pid: number;
  timeoutMs: number;
  stateFile: string;
}

/**
 * Spawn a detached bash process that kills the VM after timeout.
 * The process sleeps for the timeout duration, then verifies the VM
 * is still running with the expected PID before sending SIGTERM.
 */
export function spawnTimeoutKiller(opts: SpawnTimeoutKillerOpts): ChildProcess {
  const { vmId, pid, timeoutMs, stateFile } = opts;
  const killer = spawn(
    "bash",
    [
      "-c",
      [
        `sleep ${Math.ceil(timeoutMs / 1000)}`,
        `STATE=$(cat "${stateFile}" 2>/dev/null) || exit 0`,
        `echo "$STATE" | grep -q '"status":"running"' || exit 0`,
        `echo "$STATE" | grep -q '"pid":${pid}' || exit 0`,
        `[ -d /proc/${pid} ] || exit 0`,
        `grep -q "${vmId}" /proc/${pid}/cmdline 2>/dev/null || exit 0`,
        `kill ${pid} 2>/dev/null`,
      ].join(" && "),
    ],
    { detached: true, stdio: "ignore" },
  );
  killer.unref();
  return killer;
}
