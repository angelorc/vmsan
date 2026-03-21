import { createHash } from "node:crypto";
import {
  createReadStream,
  createWriteStream,
  existsSync,
  readFileSync,
  renameSync,
  writeFileSync,
  unlinkSync,
} from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { pipeline } from "node:stream/promises";
import { consola } from "consola";
import { mkdirSecure } from "./utils.ts";

export type RootfsType = "postgres16" | "redis7";

const DOWNLOAD_BASE_URL = "https://github.com/angelorc/vmsan/releases/download/rootfs-v1";

interface ChecksumEntry {
  sha256: string;
}

function resolveArch(): string {
  switch (process.arch) {
    case "x64":
      return "amd64";
    case "arm64":
      return "arm64";
    default:
      throw new Error(`Unsupported architecture: ${process.arch}`);
  }
}

export function getCacheDir(): string {
  return join(homedir(), ".vmsan", "rootfs");
}

function getChecksumsPath(): string {
  return join(getCacheDir(), "checksums.json");
}

function readChecksums(): Record<string, ChecksumEntry> {
  const path = getChecksumsPath();
  if (!existsSync(path)) return {};
  try {
    return JSON.parse(readFileSync(path, "utf-8"));
  } catch {
    return {};
  }
}

function writeChecksums(checksums: Record<string, ChecksumEntry>): void {
  mkdirSecure(getCacheDir());
  writeFileSync(getChecksumsPath(), JSON.stringify(checksums, null, 2));
}

export async function verifyChecksum(filePath: string, expected: string): Promise<boolean> {
  if (!existsSync(filePath)) return false;
  const actual = await computeChecksum(filePath);
  return actual === expected;
}

function computeChecksum(filePath: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const hash = createHash("sha256");
    const stream = createReadStream(filePath);
    stream.on("data", (chunk) => hash.update(chunk));
    stream.on("end", () => resolve(hash.digest("hex")));
    stream.on("error", reject);
  });
}

export async function downloadRootfs(type: RootfsType, arch?: string): Promise<void> {
  const resolvedArch = arch ?? resolveArch();
  const filename = `${type}-${resolvedArch}.ext4`;
  const url = `${DOWNLOAD_BASE_URL}/${filename}`;
  const cacheDir = getCacheDir();
  const destPath = join(cacheDir, filename);

  mkdirSecure(cacheDir);

  consola.start(`Downloading ${filename}...`);
  consola.info(`URL: ${url}`);

  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Failed to download ${filename}: ${response.status} ${response.statusText}`);
  }
  if (!response.body) {
    throw new Error(`Empty response body for ${filename}`);
  }

  const contentLength = Number(response.headers.get("content-length") ?? 0);
  const tmpPath = `${destPath}.tmp`;

  try {
    const fileStream = createWriteStream(tmpPath);
    const reader = response.body.getReader();
    let downloaded = 0;

    // Stream with throttled progress reporting (every 10%)
    let lastReportedPct = -10;
    const readable = new ReadableStream({
      async pull(controller) {
        const { done, value } = await reader.read();
        if (done) {
          controller.close();
          return;
        }
        downloaded += value.byteLength;
        if (contentLength > 0) {
          const pct = Math.round((downloaded / contentLength) * 100);
          if (pct >= lastReportedPct + 10) {
            const mb = (downloaded / 1024 / 1024).toFixed(1);
            const totalMb = (contentLength / 1024 / 1024).toFixed(1);
            consola.info(`Progress: ${mb}MB / ${totalMb}MB (${pct}%)`);
            lastReportedPct = pct;
          }
        }
        controller.enqueue(value);
      },
    });

    await pipeline(readable as never, fileStream);

    // Compute checksum and store it
    const sha256 = await computeChecksum(tmpPath);
    const checksums = readChecksums();
    checksums[filename] = { sha256 };
    writeChecksums(checksums);

    // Rename tmp to final
    renameSync(tmpPath, destPath);

    consola.success(`Downloaded ${filename} (sha256: ${sha256})`);
  } catch (error) {
    // Clean up partial download
    try {
      unlinkSync(tmpPath);
    } catch {
      // ignore cleanup errors
    }
    throw error;
  }
}

export async function getRootfsPath(type: RootfsType): Promise<string> {
  const arch = resolveArch();
  const filename = `${type}-${arch}.ext4`;
  const cacheDir = getCacheDir();
  const filePath = join(cacheDir, filename);

  // Check if cached and checksum matches
  if (existsSync(filePath)) {
    const checksums = readChecksums();
    const entry = checksums[filename];
    if (entry) {
      if (await verifyChecksum(filePath, entry.sha256)) {
        consola.debug(`Using cached rootfs: ${filePath}`);
        return filePath;
      }
      consola.warn(`Checksum mismatch for ${filename}, re-downloading`);
    } else {
      // File exists but no checksum recorded -- compute and store it
      const sha256 = await computeChecksum(filePath);
      const checksums2 = readChecksums();
      checksums2[filename] = { sha256 };
      writeChecksums(checksums2);
      consola.debug(`Using cached rootfs: ${filePath}`);
      return filePath;
    }
  }

  await downloadRootfs(type, arch);
  return filePath;
}
