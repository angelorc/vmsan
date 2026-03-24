import { existsSync, readFileSync, rmSync } from "node:fs";
import { safeKill } from "./utils.ts";

export class PidFile {
  constructor(private readonly path: string) {}

  read(): number | null {
    if (!existsSync(this.path)) return null;
    const pid = Number(readFileSync(this.path, "utf-8").trim());
    if (Number.isNaN(pid)) return null;
    if (safeKill(pid, 0)) return pid;
    rmSync(this.path, { force: true });
    return null;
  }
}
