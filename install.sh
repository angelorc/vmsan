#!/usr/bin/env bash
set -euo pipefail

# vmsan installer — downloads Firecracker, kernel, rootfs, and vmsan-agent.
# Usage: curl -fsSL https://raw.githubusercontent.com/angelorc/vmsan/main/install.sh | bash

VMSAN_DIR="${VMSAN_DIR:-$HOME/.vmsan}"
VMSAN_REPO="angelorc/vmsan"
ARCH="$(uname -m)"

# --- helpers ---

info()    { printf "\033[1;34m[info]\033[0m  %s\n" "$*"; }
success() { printf "\033[1;32m[ok]\033[0m    %s\n" "$*"; }
warn()    { printf "\033[1;33m[warn]\033[0m  %s\n" "$*"; }
error()   { printf "\033[1;31m[error]\033[0m %s\n" "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || error "Required command not found: $1"
}

download() {
  local url="$1" dest="$2"
  info "Downloading $(basename "$dest")..."
  curl -fsSL -o "$dest" "$url"
}

# --- banner ---

echo ""
echo "  vmsan installer"
echo "  Firecracker microVM sandbox toolkit"
echo "  https://github.com/$VMSAN_REPO"
echo ""

# --- checks ---

[ "$(uname -s)" = "Linux" ] || error "vmsan requires Linux"
[ "$ARCH" = "x86_64" ] || [ "$ARCH" = "aarch64" ] || error "Unsupported architecture: $ARCH"
need_cmd curl
need_cmd tar
need_cmd git

if ! command -v bun >/dev/null 2>&1; then
  info "Bun not found — installing..."
  curl -fsSL https://bun.sh/install | bash
  export BUN_INSTALL="$HOME/.bun"
  export PATH="$BUN_INSTALL/bin:$PATH"
fi

if [ ! -e /dev/kvm ]; then
  warn "/dev/kvm not found — Firecracker requires KVM. Make sure you're on a bare-metal host or a VM with nested virtualization."
fi

# --- directories ---

info "Setting up $VMSAN_DIR..."
mkdir -p "$VMSAN_DIR"/{bin,kernels,rootfs,vms,jailer,registry/rootfs,snapshots}
success "Directories created"

# --- firecracker + jailer ---

if [ -x "$VMSAN_DIR/bin/firecracker" ] && [ -x "$VMSAN_DIR/bin/jailer" ]; then
  FC_VER=$("$VMSAN_DIR/bin/firecracker" --version 2>/dev/null | head -1 | grep -oP 'v[\d.]+' || echo "unknown")
  success "Firecracker already installed ($FC_VER)"
else
  info "Fetching latest Firecracker version..."
  FC_VER=$(curl -fsSI https://github.com/firecracker-microvm/firecracker/releases/latest 2>&1 \
    | sed -n 's/.*tag\/\(v[0-9.]*\).*/\1/p')
  [ -n "$FC_VER" ] || error "Could not determine latest Firecracker version"

  FC_URL="https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VER}/firecracker-${FC_VER}-${ARCH}.tgz"
  FC_TMP=$(mktemp -d)
  trap 'rm -rf "$FC_TMP"' EXIT

  download "$FC_URL" "$FC_TMP/firecracker.tgz"
  tar -xzf "$FC_TMP/firecracker.tgz" -C "$FC_TMP"

  cp "$FC_TMP"/release-*/firecracker-*-"$ARCH" "$VMSAN_DIR/bin/firecracker"
  cp "$FC_TMP"/release-*/jailer-*-"$ARCH" "$VMSAN_DIR/bin/jailer"
  chmod +x "$VMSAN_DIR/bin/firecracker" "$VMSAN_DIR/bin/jailer"

  rm -rf "$FC_TMP"
  trap - EXIT
  success "Firecracker $FC_VER installed"
fi

# --- kernel ---

KERNEL_FILE="vmlinux-6.1"
KERNEL_PATH="$VMSAN_DIR/kernels/$KERNEL_FILE"

if [ -f "$KERNEL_PATH" ]; then
  success "Kernel already installed ($KERNEL_FILE)"
else
  KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.14/${ARCH}/vmlinux-6.1.155"
  download "$KERNEL_URL" "$KERNEL_PATH"
  success "Kernel installed ($KERNEL_FILE)"
fi

# --- rootfs (ubuntu 24.04) ---

ROOTFS_FILE="ubuntu-24.04.ext4"
ROOTFS_PATH="$VMSAN_DIR/rootfs/$ROOTFS_FILE"

if [ -f "$ROOTFS_PATH" ]; then
  success "Rootfs already installed ($ROOTFS_FILE)"
else
  # Ubuntu 24.04 is only available as squashfs on the CI bucket — download and convert.
  SQUASHFS_URL="https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.14/${ARCH}/ubuntu-24.04.squashfs"
  SQUASHFS_TMP=$(mktemp -d)
  trap 'rm -rf "$SQUASHFS_TMP"' EXIT

  download "$SQUASHFS_URL" "$SQUASHFS_TMP/ubuntu-24.04.squashfs"

  need_cmd unsquashfs
  need_cmd mkfs.ext4

  info "Converting squashfs to ext4 (this may take a minute)..."
  unsquashfs -d "$SQUASHFS_TMP/rootfs" "$SQUASHFS_TMP/ubuntu-24.04.squashfs" >/dev/null 2>&1

  # Size the image: extracted size + 512 MB headroom, minimum 1 GB
  EXTRACTED_BYTES=$(du -sb "$SQUASHFS_TMP/rootfs" | cut -f1)
  EXTRACTED_MB=$(( EXTRACTED_BYTES / 1024 / 1024 ))
  IMAGE_MB=$(( EXTRACTED_MB + 512 ))
  [ "$IMAGE_MB" -lt 1024 ] && IMAGE_MB=1024

  dd if=/dev/zero of="$ROOTFS_PATH" bs=1M count="$IMAGE_MB" status=none
  mkfs.ext4 -q "$ROOTFS_PATH"
  tune2fs -m 0 "$ROOTFS_PATH" >/dev/null 2>&1

  MOUNT_TMP=$(mktemp -d)
  sudo mount -o loop "$ROOTFS_PATH" "$MOUNT_TMP"
  sudo cp -a "$SQUASHFS_TMP/rootfs/." "$MOUNT_TMP/"
  sudo umount "$MOUNT_TMP"
  rmdir "$MOUNT_TMP"

  rm -rf "$SQUASHFS_TMP"
  trap - EXIT
  success "Rootfs installed ($ROOTFS_FILE, ${IMAGE_MB} MB)"
fi

# --- vmsan-agent ---

AGENT_PATH="$VMSAN_DIR/bin/vmsan-agent"

if [ -x "$AGENT_PATH" ]; then
  success "vmsan-agent already installed"
else
  # Download prebuilt binary from GitHub releases
  info "Fetching latest vmsan release..."
  VMSAN_VER=$(curl -fsSI "https://github.com/$VMSAN_REPO/releases/latest" 2>&1 \
    | sed -n 's/.*tag\/\(v[0-9.]*\).*/\1/p')

  if [ -n "$VMSAN_VER" ]; then
    AGENT_URL="https://github.com/$VMSAN_REPO/releases/download/${VMSAN_VER}/vmsan-agent-${ARCH}"
    download "$AGENT_URL" "$AGENT_PATH"
    chmod +x "$AGENT_PATH"
    success "vmsan-agent $VMSAN_VER installed"
  else
    warn "vmsan-agent not installed — no release found."
    warn "Create a release with: git tag v0.1.0 && git push --tags"
  fi
fi

# --- vmsan CLI ---

VMSAN_CLI="$VMSAN_DIR/cli"
VMSAN_BIN="$VMSAN_DIR/bin/vmsan"

if [ -x "$VMSAN_BIN" ]; then
  success "vmsan CLI already installed"
else
  info "Installing vmsan CLI..."
  if [ -d "$VMSAN_CLI" ]; then
    git -C "$VMSAN_CLI" pull --quiet
  else
    git clone --depth 1 "https://github.com/$VMSAN_REPO.git" "$VMSAN_CLI"
  fi

  (cd "$VMSAN_CLI" && bun install --frozen-lockfile 2>/dev/null && bun run build)

  ln -sf "$VMSAN_CLI/dist/bin/cli.mjs" "$VMSAN_BIN"
  success "vmsan CLI installed"
fi

# --- PATH ---

SHELL_RC=""
case "$(basename "${SHELL:-/bin/bash}")" in
  zsh)  SHELL_RC="$HOME/.zshrc" ;;
  *)    SHELL_RC="$HOME/.bashrc" ;;
esac

PATH_LINE='export PATH="$HOME/.vmsan/bin:$PATH"'

if ! grep -qF '.vmsan/bin' "$SHELL_RC" 2>/dev/null; then
  echo "" >> "$SHELL_RC"
  echo "# vmsan" >> "$SHELL_RC"
  echo "$PATH_LINE" >> "$SHELL_RC"
  info "Added ~/.vmsan/bin to PATH in $SHELL_RC"
else
  success "PATH already configured"
fi

# --- summary ---

echo ""
success "vmsan environment ready at $VMSAN_DIR"
echo ""
echo "  Firecracker  $VMSAN_DIR/bin/firecracker"
echo "  Jailer       $VMSAN_DIR/bin/jailer"
echo "  Kernel       $VMSAN_DIR/kernels/$KERNEL_FILE"
echo "  Rootfs       $VMSAN_DIR/rootfs/$ROOTFS_FILE"
echo "  Agent        $AGENT_PATH"
echo "  CLI          $VMSAN_BIN"
echo ""

# Show source hint if vmsan is not in current PATH
if ! command -v vmsan >/dev/null 2>&1; then
  warn "To start using vmsan, run:"
  echo ""
  echo "  source $SHELL_RC"
  echo ""
fi
