import { basename, resolve } from "node:path";
import { existsSync, readFileSync } from "node:fs";

export function resolveProject(project?: string): string {
  if (project) {
    return project;
  }

  const cwd = process.cwd();
  const tomlPath = resolve(cwd, "vmsan.toml");
  if (existsSync(tomlPath)) {
    try {
      const content = readFileSync(tomlPath, "utf-8");
      const match = content.match(/^project\s*=\s*"([^"]+)"/m);
      if (match) return match[1];
    } catch {
      // Fall through to basename
    }
  }
  return basename(cwd);
}
