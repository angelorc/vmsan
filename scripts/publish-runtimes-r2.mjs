#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { homedir, tmpdir } from "node:os";
import { join } from "node:path";
import { createReadStream } from "node:fs";

const DEFAULT_RUNTIMES = ["node22", "node24", "python3.13"];
const DEFAULT_BASE_IMAGES = {
  node22: process.env.VMSAN_RUNTIME_NODE22_IMAGE || "node:22",
  node24: process.env.VMSAN_RUNTIME_NODE24_IMAGE || "node:24",
  "python3.13": process.env.VMSAN_RUNTIME_PYTHON313_IMAGE || "python:3.13-slim",
};

function printHelp() {
  console.log(`Usage: node scripts/publish-runtimes-r2.mjs [options]

Required environment:
  R2_BUCKET                  Target R2 bucket name
  R2_ENDPOINT                S3-compatible R2 endpoint URL
  R2_PUBLIC_BASE_URL         Public download base URL (custom domain)

Optional environment:
  VMSAN_DIR                  Local vmsan dir (default: ~/.vmsan)
  VMSAN_RUNTIME_VERSION      Artifact version (default: v<package.json version>)
  R2_PREFIX                  Object prefix inside the bucket (default: runtimes)
  RUNTIMES                   Comma-separated runtime names (default: node22,node24,python3.13)
  VMSAN_RUNTIME_NODE22_IMAGE Override node22 source image reference
  VMSAN_RUNTIME_NODE24_IMAGE Override node24 source image reference
  VMSAN_RUNTIME_PYTHON313_IMAGE Override python3.13 source image reference

Options:
  --dry-run                  Print planned uploads without writing to R2
  --version <value>          Override the artifact version for this publish
  --runtime <name>           Publish only one runtime (repeatable)
  --help                     Show this message
`);
}

function fail(message) {
  console.error(message);
  process.exit(1);
}

function parseArgs(argv) {
  const args = { dryRun: false, version: "", runtimes: [] };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    switch (arg) {
      case "--dry-run":
        args.dryRun = true;
        break;
      case "--version":
        if (i + 1 >= argv.length) fail("--version requires a value");
        args.version = argv[i + 1];
        i += 1;
        break;
      case "--runtime":
        if (i + 1 >= argv.length) fail("--runtime requires a value");
        args.runtimes.push(argv[i + 1]);
        i += 1;
        break;
      case "--help":
      case "-h":
        printHelp();
        process.exit(0);
      default:
        fail(`Unknown argument: ${arg}`);
    }
  }
  return args;
}

function requireEnv(name) {
  const value = process.env[name];
  if (!value) fail(`Missing required environment variable: ${name}`);
  return value;
}

function maybeExec(command, args) {
  try {
    return execFileSync(command, args, {
      encoding: "utf8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim();
  } catch {
    return null;
  }
}

function localArch() {
  switch (process.arch) {
    case "x64":
      return "amd64";
    case "arm64":
      return "arm64";
    default:
      fail(`Unsupported architecture for runtime publishing: ${process.arch}`);
  }
}

function defaultVersion() {
  const packageJson = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  return `v${packageJson.version}`;
}

function normalizeBaseUrl(url) {
  return url.endsWith("/") ? url.slice(0, -1) : url;
}

function parseMetaFile(filePath) {
  if (!existsSync(filePath)) return {};
  const result = {};
  for (const rawLine of readFileSync(filePath, "utf8").split("\n")) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) continue;
    const separator = line.indexOf("=");
    if (separator === -1) continue;
    const key = line.slice(0, separator);
    const value = line.slice(separator + 1);
    result[key] = value;
  }
  return result;
}

async function sha256File(filePath) {
  const hash = createHash("sha256");

  await new Promise((resolve, reject) => {
    const stream = createReadStream(filePath);
    stream.on("data", (chunk) => hash.update(chunk));
    stream.on("end", resolve);
    stream.on("error", reject);
  });

  return hash.digest("hex");
}

function dockerRepoDigest(imageRef) {
  if (!imageRef) return null;
  const value = maybeExec("docker", [
    "image",
    "inspect",
    imageRef,
    "--format",
    "{{index .RepoDigests 0}}",
  ]);
  if (!value || value === "<no value>") return null;
  return value;
}

function awsBaseArgs(endpoint) {
  return ["--endpoint-url", endpoint, "--only-show-errors"];
}

function readExistingManifest(bucket, endpoint, key) {
  const output = maybeExec("aws", [
    ...awsBaseArgs(endpoint),
    "s3",
    "cp",
    `s3://${bucket}/${key}`,
    "-",
  ]);
  if (!output) return null;
  return JSON.parse(output);
}

function uploadFile(localPath, bucket, endpoint, key, contentType, cacheControl, dryRun) {
  if (dryRun) {
    console.log(`[dry-run] aws s3 cp ${localPath} s3://${bucket}/${key}`);
    return;
  }

  execFileSync(
    "aws",
    [
      ...awsBaseArgs(endpoint),
      "s3",
      "cp",
      localPath,
      `s3://${bucket}/${key}`,
      "--content-type",
      contentType,
      "--cache-control",
      cacheControl,
    ],
    { stdio: "inherit" },
  );
}

function objectKey(...parts) {
  return parts.map((part) => String(part).replace(/^\/+|\/+$/g, "")).join("/");
}

async function main() {
  const cliArgs = parseArgs(process.argv.slice(2));
  const bucket = requireEnv("R2_BUCKET");
  const endpoint = requireEnv("R2_ENDPOINT");
  const publicBaseUrl = normalizeBaseUrl(requireEnv("R2_PUBLIC_BASE_URL"));
  const prefix = (process.env.R2_PREFIX || "runtimes").replace(/^\/+|\/+$/g, "");
  const version = cliArgs.version || process.env.VMSAN_RUNTIME_VERSION || defaultVersion();
  const vmsanDir = process.env.VMSAN_DIR || join(homedir(), ".vmsan");
  const rootfsDir = join(vmsanDir, "rootfs");
  const arch = localArch();
  const platform = `linux-${arch}`;

  const runtimes =
    cliArgs.runtimes.length > 0
      ? cliArgs.runtimes
      : (process.env.RUNTIMES || DEFAULT_RUNTIMES.join(","))
          .split(",")
          .map((value) => value.trim())
          .filter(Boolean);

  const tempDir = mkdtempSync(join(tmpdir(), "vmsan-r2-publish-"));

  try {
    const manifestKey = objectKey(prefix, version, "manifest.json");
    const existingManifest = readExistingManifest(bucket, endpoint, manifestKey) || {};
    const manifest = {
      version,
      generatedAt: new Date().toISOString(),
      runtimes: { ...existingManifest.runtimes },
    };

    for (const runtime of runtimes) {
      if (!DEFAULT_BASE_IMAGES[runtime]) {
        fail(`Unsupported runtime for publishing: ${runtime}`);
      }

      const rootfsPath = join(rootfsDir, `${runtime}.ext4`);
      if (!existsSync(rootfsPath)) {
        fail(`Runtime image not found: ${rootfsPath}`);
      }

      const metaPath = join(rootfsDir, `${runtime}.meta`);
      const localMeta = parseMetaFile(metaPath);
      const baseImage = localMeta.base_image || DEFAULT_BASE_IMAGES[runtime];
      const recipeVersion = localMeta.recipe_version || "";
      const builtAt = localMeta.built_at || "";
      const artifactSha256 = await sha256File(rootfsPath);
      const artifactBytes = statSync(rootfsPath).size;
      const sourceImageDigest = dockerRepoDigest(baseImage);
      const publishedAt = new Date().toISOString();
      const keyBase = objectKey(prefix, version, runtime, platform);

      const metadata = {
        runtime,
        platform,
        arch,
        vmsanVersion: version,
        recipeVersion,
        baseImage,
        sourceImageDigest,
        artifactSha256,
        artifactBytes,
        builtAt,
        publishedAt,
      };

      const metadataPath = join(tempDir, `${runtime}-${arch}-metadata.json`);
      const shaPath = join(tempDir, `${runtime}-${arch}.sha256`);
      writeFileSync(metadataPath, `${JSON.stringify(metadata, null, 2)}\n`);
      writeFileSync(shaPath, `${artifactSha256}  rootfs.ext4\n`);

      uploadFile(
        rootfsPath,
        bucket,
        endpoint,
        objectKey(keyBase, "rootfs.ext4"),
        "application/octet-stream",
        "public, max-age=31536000, immutable",
        cliArgs.dryRun,
      );
      uploadFile(
        shaPath,
        bucket,
        endpoint,
        objectKey(keyBase, "rootfs.ext4.sha256"),
        "text/plain; charset=utf-8",
        "public, max-age=31536000, immutable",
        cliArgs.dryRun,
      );
      uploadFile(
        metadataPath,
        bucket,
        endpoint,
        objectKey(keyBase, "metadata.json"),
        "application/json",
        "public, max-age=31536000, immutable",
        cliArgs.dryRun,
      );

      manifest.runtimes[runtime] = {
        ...manifest.runtimes[runtime],
        [platform]: {
          rootfs: `${publicBaseUrl}/${objectKey(keyBase, "rootfs.ext4")}`,
          sha256: `${publicBaseUrl}/${objectKey(keyBase, "rootfs.ext4.sha256")}`,
          metadata: `${publicBaseUrl}/${objectKey(keyBase, "metadata.json")}`,
          artifactSha256,
          artifactBytes,
          recipeVersion,
          baseImage,
          sourceImageDigest,
          publishedAt,
        },
      };
    }

    const manifestPath = join(tempDir, "manifest.json");
    writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
    uploadFile(
      manifestPath,
      bucket,
      endpoint,
      manifestKey,
      "application/json",
      "public, max-age=60",
      cliArgs.dryRun,
    );

    console.log(
      `Published runtime artifacts for ${platform} at ${publicBaseUrl}/${prefix}/${version}`,
    );
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
}

await main();
