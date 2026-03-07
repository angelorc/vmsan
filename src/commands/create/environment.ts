import { accessSync, constants, existsSync, readFileSync, readdirSync } from "node:fs";
import { connect } from "node:net";
import { join } from "node:path";
import type { VmsanPaths } from "../../paths.ts";
import type { Runtime } from "./types.ts";
import {
  missingBinaryError,
  noKernelDirError,
  noKernelError,
  noRootfsDirError,
  noExt4RootfsError,
  snapshotNotFoundError,
  kvmUnavailableError,
} from "../../errors/index.ts";
import { socketTimeoutError, SetupError } from "../../errors/index.ts";

export function validateEnvironment(baseDir: string): void {
  const firecrackerPath = join(baseDir, "bin", "firecracker");
  const jailerPath = join(baseDir, "bin", "jailer");

  if (!existsSync(firecrackerPath)) {
    throw missingBinaryError("Firecracker", firecrackerPath);
  }
  if (!existsSync(jailerPath)) {
    throw missingBinaryError("Jailer", jailerPath);
  }

  try {
    accessSync("/dev/kvm", constants.R_OK | constants.W_OK);
  } catch {
    throw kvmUnavailableError();
  }
}

export function findKernel(baseDir: string): string {
  const kernelDir = join(baseDir, "kernels");
  if (!existsSync(kernelDir)) {
    throw noKernelDirError();
  }
  const files = readdirSync(kernelDir).filter((fileName) => fileName.startsWith("vmlinux"));
  if (files.length === 0) {
    throw noKernelError();
  }
  return join(kernelDir, files.sort().at(-1)!);
}

const RUNTIME_ROOTFS_MAP: Record<Exclude<Runtime, "base">, string> = {
  node22: "node22.ext4",
  node24: "node24.ext4",
  "python3.13": "python3.13.ext4",
};

const BASE_ROOTFS_FILENAMES = ["ubuntu-24.04.ext4"];

export function findRuntimeRootfs(runtime: Exclude<Runtime, "base">, baseDir: string): string {
  const filename = RUNTIME_ROOTFS_MAP[runtime];
  const rootfsPath = join(baseDir, "rootfs", filename);
  if (!existsSync(rootfsPath)) {
    throw new SetupError("ERR_SETUP_NO_EXT4_ROOTFS", {
      message: `Runtime "${runtime}" rootfs not found at ${rootfsPath}`,
      fix: 'Run "curl -fsSL https://vmsan.dev/install | bash" to build runtime images.',
    });
  }
  return rootfsPath;
}

export function findBaseRootfs(baseDir: string): string {
  const rootfsDir = join(baseDir, "rootfs");
  if (!existsSync(rootfsDir)) {
    throw noRootfsDirError();
  }

  for (const filename of BASE_ROOTFS_FILENAMES) {
    const rootfsPath = join(rootfsDir, filename);
    if (existsSync(rootfsPath)) {
      return rootfsPath;
    }
  }

  const files = readdirSync(rootfsDir).filter((fileName) => fileName.endsWith(".ext4"));
  const runtimeFilenames = new Set(Object.values(RUNTIME_ROOTFS_MAP));
  const baseFiles = files.filter((fileName) => !runtimeFilenames.has(fileName));
  if (baseFiles.length === 0) {
    throw noExt4RootfsError();
  }
  return join(rootfsDir, baseFiles.sort().at(-1)!);
}

export function findRootfs(baseDir: string): string {
  const rootfsDir = join(baseDir, "rootfs");
  if (!existsSync(rootfsDir)) {
    throw noRootfsDirError();
  }
  const files = readdirSync(rootfsDir).filter((fileName) => fileName.endsWith(".ext4"));
  if (files.length === 0) {
    throw noExt4RootfsError();
  }
  return join(rootfsDir, files.sort().at(-1)!);
}

export async function waitForSocket(socketPath: string, timeoutMs: number = 5000): Promise<void> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    if (existsSync(socketPath)) {
      const isConnectable = await new Promise<boolean>((resolve) => {
        const socket = connect(socketPath);
        socket.on("connect", () => {
          socket.destroy();
          resolve(true);
        });
        socket.on("error", () => resolve(false));
      });
      if (isConnectable) return;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw socketTimeoutError(socketPath);
}

export function getVmPid(vmId: string): number | null {
  try {
    const entries = readdirSync("/proc").filter((entry) => /^\d+$/.test(entry));
    for (const entry of entries) {
      try {
        const cmdline = readFileSync(`/proc/${entry}/cmdline`, "utf-8");
        if (cmdline.includes("firecracker") && cmdline.includes(vmId)) {
          return Number(entry);
        }
      } catch {
        // Process exited between readdir and readFileSync
      }
    }
  } catch {
    // /proc may not be readable
  }
  return null;
}

export function getVmJailerPid(vmId: string): number | null {
  try {
    const entries = readdirSync("/proc").filter((entry) => /^\d+$/.test(entry));
    for (const entry of entries) {
      try {
        const cmdline = readFileSync(`/proc/${entry}/cmdline`, "utf-8");
        if (cmdline.includes("jailer") && cmdline.includes(vmId)) {
          return Number(entry);
        }
      } catch {
        // Process exited between readdir and readFileSync
      }
    }
  } catch {
    // /proc may not be readable
  }
  return null;
}

export function assertSnapshotExists(snapshotId: string, paths: VmsanPaths): void {
  const snapshotDir = join(paths.snapshotsDir, snapshotId);
  if (
    !existsSync(join(snapshotDir, "snapshot_file")) ||
    !existsSync(join(snapshotDir, "mem_file"))
  ) {
    throw snapshotNotFoundError(snapshotId);
  }
}
