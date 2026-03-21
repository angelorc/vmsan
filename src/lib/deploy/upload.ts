import { readdirSync, statSync, existsSync, readFileSync } from "node:fs";
import { join, relative, posix } from "node:path";
import consola from "consola";
import type { AgentClient, WriteFileEntry } from "../../services/agent.ts";
import { toError } from "../utils.ts";

export interface UploadOptions {
  /** Source directory to upload */
  sourceDir: string;
  /** Agent client for the target VM */
  agent: AgentClient;
  /** Target directory in VM (default: /app) */
  targetDir?: string;
}

export interface UploadResult {
  filesUploaded: number;
  bytesUploaded: number;
  durationMs: number;
}

/** Max batch size in bytes (50MB) */
const MAX_BATCH_BYTES = 50 * 1024 * 1024;

/** Directories always excluded from upload */
const ALWAYS_SKIP = new Set([".git", "node_modules", ".vmsan"]);

/**
 * Upload source directory contents to a VM via the agent file transfer API.
 */
export async function uploadSource(opts: UploadOptions): Promise<UploadResult> {
  const { sourceDir, agent, targetDir = "/app" } = opts;
  const startTime = Date.now();

  consola.start(`Uploading source from ${sourceDir} to ${targetDir}`);

  const ignorePatterns = loadIgnorePatterns(sourceDir);
  const files = collectFiles(sourceDir, sourceDir, ignorePatterns);

  if (files.length === 0) {
    consola.warn("No files to upload");
    return { filesUploaded: 0, bytesUploaded: 0, durationMs: Date.now() - startTime };
  }

  let totalBytes = 0;
  for (const f of files) {
    totalBytes += f.content.length;
  }

  consola.debug(`Collected ${files.length} files (${formatBytes(totalBytes)})`);

  try {
    if (totalBytes <= MAX_BATCH_BYTES) {
      await agent.writeFiles(files, targetDir);
    } else {
      const batches = createBatches(files, MAX_BATCH_BYTES);
      consola.debug(`Uploading in ${batches.length} batches`);
      for (let i = 0; i < batches.length; i++) {
        consola.debug(`Batch ${i + 1}/${batches.length} (${batches[i].length} files)`);
        await agent.writeFiles(batches[i], targetDir);
      }
    }

    const durationMs = Date.now() - startTime;
    consola.success(
      `Uploaded ${files.length} files (${formatBytes(totalBytes)}) in ${Math.round(durationMs / 1000)}s`,
    );

    return { filesUploaded: files.length, bytesUploaded: totalBytes, durationMs };
  } catch (err) {
    consola.error(`Upload failed: ${toError(err).message}`);
    throw err;
  }
}

/**
 * Load ignore patterns from .gitignore and .vmsanignore files.
 */
function loadIgnorePatterns(rootDir: string): string[] {
  const patterns: string[] = [];
  for (const filename of [".gitignore", ".vmsanignore"]) {
    const filepath = join(rootDir, filename);
    if (existsSync(filepath)) {
      const content = readFileSync(filepath, "utf-8");
      for (const line of content.split("\n")) {
        const trimmed = line.trim();
        // Skip empty lines, comments, and negations
        if (!trimmed || trimmed.startsWith("#") || trimmed.startsWith("!")) continue;
        patterns.push(trimmed);
      }
    }
  }
  return patterns;
}

/**
 * Recursively collect files from a directory, respecting ignore patterns.
 */
function collectFiles(dir: string, rootDir: string, ignorePatterns: string[]): WriteFileEntry[] {
  const entries: WriteFileEntry[] = [];

  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    const relPath = relative(rootDir, fullPath);
    // Use posix separators for tar paths
    const posixRelPath = relPath.split("/").join(posix.sep);

    if (entry.isDirectory()) {
      if (ALWAYS_SKIP.has(entry.name)) continue;
      if (isIgnored(posixRelPath + "/", ignorePatterns)) continue;
      entries.push(...collectFiles(fullPath, rootDir, ignorePatterns));
    } else if (entry.isFile()) {
      if (isIgnored(posixRelPath, ignorePatterns)) continue;
      entries.push({
        path: posixRelPath,
        content: readFileSync(fullPath),
      });
    }
  }

  return entries;
}

/**
 * Check if a path matches any ignore pattern.
 */
function isIgnored(relPath: string, patterns: string[]): boolean {
  for (const pattern of patterns) {
    if (matchPattern(relPath, pattern)) return true;
  }
  return false;
}

/**
 * Simple glob-style pattern matching for ignore files.
 * Supports: *, **, trailing / for directories, and path prefixes.
 */
function matchPattern(relPath: string, pattern: string): boolean {
  let pat = pattern;

  // Directory-only pattern: only match paths ending with /
  const dirOnly = pat.endsWith("/");
  if (dirOnly) {
    pat = pat.slice(0, -1);
    if (!relPath.endsWith("/")) return false;
  }

  const testPath = relPath.endsWith("/") ? relPath.slice(0, -1) : relPath;

  // If pattern has no slash (except trailing), match against any path component
  if (!pat.includes("/")) {
    const parts = testPath.split("/");
    return parts.some((part) => globMatch(part, pat));
  }

  // Pattern with slashes: match from root
  const cleanPat = pat.startsWith("/") ? pat.slice(1) : pat;
  return globMatch(testPath, cleanPat);
}

/**
 * Match a string against a glob pattern supporting * and **.
 */
function globMatch(str: string, pattern: string): boolean {
  // Convert glob to regex
  let regexStr = "^";
  let i = 0;
  while (i < pattern.length) {
    if (pattern[i] === "*" && pattern[i + 1] === "*") {
      // ** matches any number of directories
      if (pattern[i + 2] === "/") {
        regexStr += "(?:.+/)?";
        i += 3;
      } else {
        regexStr += ".*";
        i += 2;
      }
    } else if (pattern[i] === "*") {
      // * matches anything except /
      regexStr += "[^/]*";
      i++;
    } else if (pattern[i] === "?") {
      regexStr += "[^/]";
      i++;
    } else {
      regexStr += escapeRegex(pattern[i]);
      i++;
    }
  }
  regexStr += "$";

  return new RegExp(regexStr).test(str);
}

function escapeRegex(char: string): string {
  return char.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Split files into batches that each stay under the byte limit.
 */
function createBatches(files: WriteFileEntry[], maxBytes: number): WriteFileEntry[][] {
  const batches: WriteFileEntry[][] = [];
  let currentBatch: WriteFileEntry[] = [];
  let currentSize = 0;

  for (const file of files) {
    if (currentBatch.length > 0 && currentSize + file.content.length > maxBytes) {
      batches.push(currentBatch);
      currentBatch = [];
      currentSize = 0;
    }
    currentBatch.push(file);
    currentSize += file.content.length;
  }

  if (currentBatch.length > 0) {
    batches.push(currentBatch);
  }

  return batches;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
