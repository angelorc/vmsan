import { existsSync, readFileSync, readdirSync } from "node:fs";
import { connect } from "node:net";
import { join } from "node:path";
import type { VmsanPaths } from "../../paths.ts";
import {
  missingBinaryError,
  noKernelDirError,
  noKernelError,
  noRootfsDirError,
  noExt4RootfsError,
  snapshotNotFoundError,
} from "../../errors/index.ts";
import { socketTimeoutError } from "../../errors/index.ts";

export function validateEnvironment(baseDir: string): void {
  const firecrackerPath = join(baseDir, "bin", "firecracker");
  const jailerPath = join(baseDir, "bin", "jailer");

  if (!existsSync(firecrackerPath)) {
    throw missingBinaryError("Firecracker", firecrackerPath);
  }
  if (!existsSync(jailerPath)) {
    throw missingBinaryError("Jailer", jailerPath);
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
