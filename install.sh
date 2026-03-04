#!/usr/bin/env bash
set -euo pipefail

# vmsan installer — downloads Firecracker, kernel, rootfs, and vmsan-agent.
# Usage:
#   Install: curl -fsSL https://vmsan.dev/install | bash
#   Uninstall: curl -fsSL https://vmsan.dev/install | bash -s -- --uninstall

VMSAN_DIR="${VMSAN_DIR:-$HOME/.vmsan}"
VMSAN_REPO="angelorc/vmsan"
VMSAN_REF="${VMSAN_REF:-}"
ARCH="$(uname -m)"
CLOUDFLARED_VERSION="${CLOUDFLARED_VERSION:-2026.2.0}"

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

go_arch() {
  case "$ARCH" in
    x86_64)  echo "amd64" ;;
    aarch64) echo "arm64" ;;
    *)       error "Unsupported architecture: $ARCH" ;;
  esac
}

# --- uninstall ---

if [ "${1:-}" = "--uninstall" ]; then
  echo ""
  echo "  vmsan uninstaller"
  echo ""

  if command -v vmsan >/dev/null 2>&1; then
    info "Removing vmsan CLI (npm global)..."
    npm uninstall -g vmsan
    success "vmsan CLI removed"
  else
    success "vmsan CLI not installed"
  fi

  if [ -f "$VMSAN_DIR/cloudflare/cloudflared.pid" ]; then
    CF_PID="$(cat "$VMSAN_DIR/cloudflare/cloudflared.pid" 2>/dev/null || true)"
    if [ -n "${CF_PID:-}" ]; then
      kill -TERM "$CF_PID" 2>/dev/null || true
    fi
  fi

  if [ -d "$VMSAN_DIR" ]; then
    info "Removing $VMSAN_DIR..."
    rm -rf "$VMSAN_DIR"
    success "$VMSAN_DIR removed"
  else
    success "$VMSAN_DIR not found"
  fi

  echo ""
  success "vmsan has been uninstalled"
  echo ""
  exit 0
fi

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

# --- prerequisites ---

install_pkg() {
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq && apt-get install -y -qq "$@" >/dev/null 2>&1
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y -q "$@" >/dev/null 2>&1
  elif command -v yum >/dev/null 2>&1; then
    yum install -y -q "$@" >/dev/null 2>&1
  else
    error "Cannot install packages: no supported package manager (apt-get, dnf, yum)"
  fi
}

MISSING_PKGS=()
command -v unzip       >/dev/null 2>&1 || MISSING_PKGS+=(unzip)
command -v unsquashfs  >/dev/null 2>&1 || MISSING_PKGS+=(squashfs-tools)
command -v mkfs.ext4   >/dev/null 2>&1 || MISSING_PKGS+=(e2fsprogs)

if [ ${#MISSING_PKGS[@]} -gt 0 ]; then
  info "Installing prerequisites: ${MISSING_PKGS[*]}..."
  install_pkg "${MISSING_PKGS[@]}"
  success "Prerequisites installed"
fi

if ! command -v node >/dev/null 2>&1; then
  info "Node.js not found — installing Node.js 22 via NodeSource..."
  curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
  apt-get install -y -qq nodejs >/dev/null 2>&1
  success "Node.js $(node --version) installed"
fi

if [ ! -e /dev/kvm ]; then
  warn "/dev/kvm not found — Firecracker requires KVM. Make sure you're on a bare-metal host or a VM with nested virtualization."
fi

# --- directories ---

info "Setting up $VMSAN_DIR..."
mkdir -p "$VMSAN_DIR"/{bin,kernels,rootfs,vms,jailer,registry/rootfs,snapshots,cloudflare}
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
  mount -o loop "$ROOTFS_PATH" "$MOUNT_TMP"
  cp -a "$SQUASHFS_TMP/rootfs/." "$MOUNT_TMP/"
  umount "$MOUNT_TMP"
  rmdir "$MOUNT_TMP"

  rm -rf "$SQUASHFS_TMP"
  trap - EXIT
  success "Rootfs installed ($ROOTFS_FILE, ${IMAGE_MB} MB)"
fi

# --- latest release tag ---

info "Fetching latest release tag..."
LATEST_TAG=$(curl -fsSL "https://api.github.com/repos/$VMSAN_REPO/releases" | grep -oP '"tag_name":\s*"\K[^"]+' | head -1)
[ -n "$LATEST_TAG" ] || error "Could not determine latest release tag"
success "Latest release: $LATEST_TAG"

# --- vmsan CLI ---

if [ -n "$VMSAN_REF" ]; then
  info "Installing vmsan CLI from branch/ref: $VMSAN_REF..."
  VMSAN_SRC="$VMSAN_DIR/src"
  rm -rf "$VMSAN_SRC"
  mkdir -p "$VMSAN_SRC"
  curl -fsSL "https://github.com/$VMSAN_REPO/archive/refs/heads/${VMSAN_REF}.tar.gz" \
    | tar -xz --strip-components=1 -C "$VMSAN_SRC"
  (cd "$VMSAN_SRC" && npm install --ignore-scripts && npx obuild)
  npm install -g "$VMSAN_SRC"
  VMSAN_VER=$(vmsan --version 2>/dev/null || echo "unknown")
  success "vmsan CLI installed from $VMSAN_REF ($VMSAN_VER)"
elif command -v vmsan >/dev/null 2>&1; then
  VMSAN_VER=$(vmsan --version 2>/dev/null || echo "unknown")
  success "vmsan CLI already installed ($VMSAN_VER)"
else
  info "Installing vmsan CLI via npm..."
  npm install -g vmsan@latest
  VMSAN_VER=$(vmsan --version 2>/dev/null || echo "unknown")
  success "vmsan CLI installed ($VMSAN_VER)"
fi

# --- vmsan-agent ---

AGENT_PATH="$VMSAN_DIR/bin/vmsan-agent"

if [ -x "$AGENT_PATH" ]; then
  success "vmsan-agent already installed"
else
  AGENT_URL="https://github.com/$VMSAN_REPO/releases/download/${LATEST_TAG}/vmsan-agent-${ARCH}"
  download "$AGENT_URL" "$AGENT_PATH"
  chmod +x "$AGENT_PATH"
  success "vmsan-agent ${LATEST_TAG} installed"
fi

# --- cloudflared ---

CLOUDFLARED_PATH="$VMSAN_DIR/bin/cloudflared"

if [ -x "$CLOUDFLARED_PATH" ]; then
  success "cloudflared already installed"
else
  CLOUDFLARED_ARCH=$(go_arch)
  CLOUDFLARED_URL="https://github.com/cloudflare/cloudflared/releases/download/${CLOUDFLARED_VERSION}/cloudflared-linux-${CLOUDFLARED_ARCH}"
  download "$CLOUDFLARED_URL" "$CLOUDFLARED_PATH"
  chmod +x "$CLOUDFLARED_PATH"
  success "cloudflared $CLOUDFLARED_VERSION installed"
fi

# --- cloudflare configuration ---

CLOUDFLARE_JSON="$VMSAN_DIR/cloudflare/cloudflare.json"

if [ -f "$CLOUDFLARE_JSON" ]; then
  success "Cloudflare already configured"
else
  if (exec </dev/tty) 2>/dev/null; then
    echo ""
    echo "  ┌─────────────────────────────────────────────────────────────┐"
    echo "  │  Cloudflare Tunnel (optional)                              │"
    echo "  │                                                            │"
    echo "  │  vmsan can expose VMs via Cloudflare Tunnels.              │"
    echo "  │  You need a Cloudflare API token and a domain managed      │"
    echo "  │  by Cloudflare.                                            │"
    echo "  └─────────────────────────────────────────────────────────────┘"
    echo ""
    printf "  Configure Cloudflare now? [y/N] "
    read -r CF_SETUP </dev/tty

    if [ "$CF_SETUP" = "y" ] || [ "$CF_SETUP" = "Y" ]; then
      echo ""
      echo "  Create an API token at:"
      echo "  https://dash.cloudflare.com/profile/api-tokens"
      echo ""
      echo "  Required permissions:"
      echo "    - Account / Cloudflare Tunnel / Edit"
      echo "    - Zone / DNS / Edit"
      echo ""
      printf "  Cloudflare API token: "
      read -r CF_TOKEN </dev/tty

      if [ -z "$CF_TOKEN" ]; then
        warn "No token provided — skipping Cloudflare configuration"
      else
        printf "  Cloudflare domain (e.g. example.com): "
        read -r CF_DOMAIN </dev/tty

        if [ -z "$CF_DOMAIN" ]; then
          warn "No domain provided — skipping Cloudflare configuration"
        else
          # Verify token via Cloudflare API
          info "Verifying Cloudflare API token..."
          CF_VERIFY=$(curl -fsSL -H "Authorization: Bearer $CF_TOKEN" \
            "https://api.cloudflare.com/client/v4/user/tokens/verify" 2>/dev/null || echo "")

          if echo "$CF_VERIFY" | grep -q '"success":true'; then
            cat > "$CLOUDFLARE_JSON" <<EOF
{
  "token": "$CF_TOKEN",
  "domain": "$CF_DOMAIN"
}
EOF
            chmod 600 "$CLOUDFLARE_JSON"
            success "Cloudflare configured (domain: $CF_DOMAIN)"
            info "VMs will be exposed via Cloudflare Tunnel (direct port forwarding disabled)"
          else
            warn "Token verification failed — skipping Cloudflare configuration"
          fi
        fi
      fi
    else
      info "Skipping Cloudflare configuration"
    fi
  fi
fi

# --- summary ---

CF_STATUS="not configured"
[ -f "$VMSAN_DIR/cloudflare/cloudflare.json" ] && CF_STATUS="configured"

echo ""
success "vmsan environment ready at $VMSAN_DIR"
echo ""
echo "  Firecracker  $VMSAN_DIR/bin/firecracker"
echo "  Jailer       $VMSAN_DIR/bin/jailer"
echo "  Kernel       $VMSAN_DIR/kernels/$KERNEL_FILE"
echo "  Rootfs       $VMSAN_DIR/rootfs/$ROOTFS_FILE"
echo "  Agent        $AGENT_PATH"
echo "  cloudflared  $CLOUDFLARED_PATH ($CF_STATUS)"
echo "  CLI          $(command -v vmsan 2>/dev/null || echo 'vmsan (npm global)')"
echo ""
if [ "$CF_STATUS" = "configured" ]; then
  echo "  Tunnel mode active — VMs exposed via Cloudflare (no DNAT)."
  echo ""
elif [ "$CF_STATUS" = "not configured" ]; then
  echo "  To configure Cloudflare later, re-run this installer."
  echo ""
fi
