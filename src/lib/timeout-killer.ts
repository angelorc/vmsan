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
  const timeoutSec = String(Math.ceil(timeoutMs / 1000));
  const killer = spawn(
    "bash",
    [
      "-c",
      [
        'sleep "$1"',
        'STATE=$(cat -- "$2" 2>/dev/null) || exit 0',
        'echo "$STATE" | grep -q \'"status":"running"\' || exit 0',
        'echo "$STATE" | grep -q ""pid":$3" || exit 0',
        '[ -d "/proc/$3" ] || exit 0',
        'grep -aq -- "$4" "/proc/$3/cmdline" 2>/dev/null || exit 0',
        'kill -- "$3" 2>/dev/null',
      ].join(" && "),
      "bash",
      timeoutSec,
      stateFile,
      String(pid),
      vmId,
    ],
    { detached: true, stdio: "ignore" },
  );
  killer.unref();
  return killer;
}
