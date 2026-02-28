import { randomBytes } from "node:crypto";
import { execSync } from "node:child_process";
import { chmodSync, mkdirSync, writeFileSync } from "node:fs";
import { stripAnsi } from "consola/utils";
import { invalidDurationError } from "../errors/index.ts";

/**
 * Send a signal to a process. Returns true if delivered, false if
 * the process is already dead (ESRCH). Falls back to sudo for
 * root-owned processes (EPERM). Re-throws unexpected errors.
 */
export function safeKill(pid: number, signal: NodeJS.Signals | 0 = "SIGTERM"): boolean {
  try {
    process.kill(pid, signal);
    return true;
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    if (code === "ESRCH") return false;
    if (code === "EPERM") {
      try {
        const sig = typeof signal === "string" ? signal.replace("SIG", "") : String(signal);
        execSync(`sudo kill -${sig} ${pid}`, { stdio: "pipe" });
        return true;
      } catch {
        return false;
      }
    }
    throw error;
  }
}

/**
 * Check whether a process is alive via signal 0.
 * Returns false only for ESRCH (definitely dead).
 * Returns true for EPERM (alive but owned by another user).
 */
export function isProcessAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ESRCH") return false;
    return true; // EPERM = exists but no permission
  }
}

export function generateVmId(): string {
  return `vm-${randomBytes(4).toString("hex")}`;
}

export function parseDuration(input: string): number {
  // Plain number = minutes
  const num = Number(input);
  if (!Number.isNaN(num) && String(num) === input) {
    return num * 60 * 1000;
  }

  const regex = /(\d+)\s*(d|h|m|s)/gi;
  let totalMs = 0;
  let matched = false;
  let match;

  while ((match = regex.exec(input)) !== null) {
    matched = true;
    const value = Number(match[1]);
    const unit = match[2].toLowerCase();

    switch (unit) {
      case "d":
        totalMs += value * 24 * 60 * 60 * 1000;
        break;
      case "h":
        totalMs += value * 60 * 60 * 1000;
        break;
      case "m":
        totalMs += value * 60 * 1000;
        break;
      case "s":
        totalMs += value * 1000;
        break;
    }
  }

  if (!matched) {
    throw invalidDurationError(input);
  }

  return totalMs;
}

export function mkdirSecure(path: string): void {
  mkdirSync(path, { recursive: true, mode: 0o700 });
  try {
    chmodSync(path, 0o700);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
  }
}

export function writeSecure(path: string, contents: string): void {
  writeFileSync(path, contents, { mode: 0o600 });
  try {
    chmodSync(path, 0o600);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
  }
}

const TIME_UNITS: [number, string, string][] = [
  [86400000, "day", "days"],
  [3600000, "hour", "hours"],
  [60000, "minute", "minutes"],
  [1000, "second", "seconds"],
];

export function timeAgo(date: string | Date): string {
  const ms = Date.now() - new Date(date).getTime();
  if (ms < 30000) return "just now";
  for (const [unit, singular, plural] of TIME_UNITS) {
    const value = Math.floor(ms / unit);
    if (value >= 1) return `${value} ${value === 1 ? singular : plural} ago`;
  }
  return "just now";
}

export function timeRemaining(date: string | Date): string {
  const ms = new Date(date).getTime() - Date.now();
  if (ms <= 0) return "expired";
  for (const [unit, singular, plural] of TIME_UNITS) {
    const value = Math.floor(ms / unit);
    if (value >= 1) return `in ${value} ${value === 1 ? singular : plural}`;
  }
  return "expired";
}

export function table<T>(opts: {
  rows: T[];
  columns: Record<string, { value: (row: T) => string | number; color?: (row: T) => string }>;
}): string {
  const titles = Object.keys(opts.columns);
  const visibleLength = (value: string) => stripAnsi(value).length;
  const maxWidths: number[] = titles.map((title) => visibleLength(title));

  const data = opts.rows.map((row) => {
    return titles.map((title, i) => {
      let value = String(opts.columns[title].value(row));
      const colorFn = opts.columns[title].color;
      if (colorFn) value = colorFn(row);
      const width = visibleLength(value);
      if (width > maxWidths[i]) maxWidths[i] = width;
      return value;
    });
  });

  const padded = (value: string, i: number) => {
    const padding = maxWidths[i] - visibleLength(value);
    return padding > 0 ? `${value}${" ".repeat(padding)}` : value;
  };

  const sep = "   ";
  return [
    `\x1b[1m${titles.map(padded).join(sep)}\x1b[0m`,
    ...data.map((row) => row.map(padded).join(sep)),
  ].join("\n");
}
