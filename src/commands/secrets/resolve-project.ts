import { basename, resolve } from "node:path";
import { existsSync } from "node:fs";

export function resolveProject(): string {
  const cwd = process.cwd();
  const tomlPath = resolve(cwd, "vmsan.toml");
  if (existsSync(tomlPath)) {
    return basename(cwd);
  }
  return basename(cwd);
}
