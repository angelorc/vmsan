import type { SetupErrorCode } from "./codes.ts";
import { VmsanError } from "./base.ts";

export class SetupError extends VmsanError {
  constructor(code: SetupErrorCode, options: { message: string; fix?: string }) {
    super(code, options);
    this.name = "SetupError";
  }
}

const INSTALL_FIX = `Run the install script to set up all dependencies:\n\ncurl -fsSL https://raw.githubusercontent.com/angelorc/vmsan/main/install.sh | bash`;

export const missingBinaryError = (binary: string, path: string): SetupError =>
  new SetupError("ERR_SETUP_MISSING_BINARY", {
    message: `${binary} not found at ${path}`,
    fix: INSTALL_FIX,
  });

export const noKernelDirError = (): SetupError =>
  new SetupError("ERR_SETUP_NO_KERNEL_DIR", {
    message: "No kernels directory found.",
    fix: INSTALL_FIX,
  });

export const noKernelError = (): SetupError =>
  new SetupError("ERR_SETUP_NO_KERNEL", {
    message: "No kernel found in ~/.vmsan/kernels/.",
    fix: INSTALL_FIX,
  });

export const noRootfsDirError = (): SetupError =>
  new SetupError("ERR_SETUP_NO_ROOTFS_DIR", {
    message: "No rootfs directory found.",
    fix: INSTALL_FIX,
  });

export const noExt4RootfsError = (): SetupError =>
  new SetupError("ERR_SETUP_NO_EXT4_ROOTFS", {
    message: "No ext4 rootfs found in ~/.vmsan/rootfs/.",
    fix: INSTALL_FIX,
  });
