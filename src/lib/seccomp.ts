import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, copyFileSync, statSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { consola } from "consola";
import type { VmsanPaths } from "../paths.ts";

const _dirname = dirname(fileURLToPath(import.meta.url));

const VALID_ARCHES = ["x86_64", "aarch64"] as const;
const MAX_FILTER_SIZE = 1_048_576; // 1 MB

/**
 * Compile a Firecracker seccomp JSON filter to BPF using seccompiler-bin.
 * Falls back to using the JSON filter directly if seccompiler-bin is not available.
 */
export function compileSeccompFilter(jsonPath: string, outputPath: string, arch?: string): void {
  const targetArch = arch ?? "x86_64";
  if (!VALID_ARCHES.includes(targetArch as (typeof VALID_ARCHES)[number])) {
    throw new Error(
      `unsupported seccomp arch: ${targetArch} (allowed: ${VALID_ARCHES.join(", ")})`,
    );
  }

  // Validate file size before passing to compiler
  const stat = statSync(jsonPath);
  if (stat.size > MAX_FILTER_SIZE) {
    throw new Error(`seccomp filter too large: ${stat.size} bytes (max ${MAX_FILTER_SIZE})`);
  }

  mkdirSync(dirname(outputPath), { recursive: true });
  execFileSync(
    "seccompiler-bin",
    ["--input-file", jsonPath, "--target-arch", targetArch, "--output-file", outputPath],
    { stdio: "pipe" },
  );
}

/**
 * Ensure a seccomp filter is available for Firecracker.
 *
 * 1. If a compiled BPF exists at paths.seccompDir/default.bpf, return it.
 * 2. If the JSON source exists, try to compile it; return BPF path on success.
 * 3. If compilation fails (seccompiler-bin not installed), return null
 *    (Firecracker requires compiled BPF, not raw JSON).
 * 4. If no filter source exists at all, return null.
 */
export function ensureSeccompFilter(paths: VmsanPaths): string | null {
  const bpfPath = join(paths.seccompDir, "default.bpf");

  if (existsSync(bpfPath)) {
    consola.debug(`seccomp: using compiled BPF filter at ${bpfPath}`);
    return bpfPath;
  }

  const bundledJson = join(dirname(dirname(_dirname)), "seccomp", "default.json");
  const userJson = paths.seccompFilter;

  let sourceJson: string | null = null;
  try {
    // Warn if user-writable filter is group/world writable
    const mode = statSync(userJson).mode;
    if (mode & 0o022) {
      consola.warn(
        `seccomp: filter at ${userJson} is group/world writable (mode ${(mode & 0o777).toString(8)}); consider restricting permissions`,
      );
    }
    consola.debug(`seccomp: using user filter at ${userJson}`);
    sourceJson = userJson;
  } catch {
    // userJson does not exist, fall through to bundled
  }
  if (!sourceJson && existsSync(bundledJson)) {
    // Copy bundled filter to user dir for future use
    mkdirSync(paths.seccompDir, { recursive: true });
    copyFileSync(bundledJson, userJson);
    consola.debug(`seccomp: copied bundled filter to ${userJson}`);
    sourceJson = userJson;
  }

  if (!sourceJson) return null;

  // Try to compile JSON → BPF
  try {
    compileSeccompFilter(sourceJson, bpfPath);
    consola.debug(`seccomp: compiled BPF filter at ${bpfPath}`);
    return bpfPath;
  } catch {
    // seccompiler-bin not available; Firecracker requires compiled BPF — cannot use JSON directly.
    consola.warn(
      "seccomp: BPF compilation failed (seccompiler-bin not available?); seccomp filtering disabled. Install seccompiler-bin for seccomp support.",
    );
    return null;
  }
}
