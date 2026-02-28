import type { SetupErrorCode } from "./codes.ts";
import { VmsanError } from "./base.ts";

export class SetupError extends VmsanError {
  constructor(code: SetupErrorCode, options: { message: string; fix?: string }) {
    super(code, options);
    this.name = "SetupError";
  }
}

export const missingBinaryError = (binary: string, path: string): SetupError =>
  new SetupError("ERR_SETUP_MISSING_BINARY", {
    message: `${binary} not found at ${path}`,
    fix: "Ensure Firecracker and Jailer are installed in the bin directory.",
  });

export const noKernelDirError = (): SetupError =>
  new SetupError("ERR_SETUP_NO_KERNEL_DIR", {
    message: "No kernels directory found.",
    fix: "Place vmlinux kernel images in the kernels directory.",
  });

export const noKernelError = (): SetupError =>
  new SetupError("ERR_SETUP_NO_KERNEL", {
    message: "No kernel found.",
    fix: "Place vmlinux kernel images in the kernels directory.",
  });

export const noRootfsDirError = (): SetupError =>
  new SetupError("ERR_SETUP_NO_ROOTFS_DIR", {
    message: "No rootfs directory found.",
    fix: "Place ext4 rootfs images in the rootfs directory.",
  });

export const noExt4RootfsError = (): SetupError =>
  new SetupError("ERR_SETUP_NO_EXT4_ROOTFS", {
    message: "No ext4 rootfs found.",
    fix: "Place ext4 rootfs images in the rootfs directory.",
  });
