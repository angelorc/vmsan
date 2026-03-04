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

  # Stop running VMs before removing CLI
  if [ -d "$VMSAN_DIR/vms" ]; then
    for state_file in "$VMSAN_DIR"/vms/*.json; do
      [ -f "$state_file" ] || continue
      VM_ID="$(grep -oP '"id"\s*:\s*"\K[^"]+' "$state_file" 2>/dev/null || true)"
      VM_PID="$(grep -oP '"pid"\s*:\s*\K\d+' "$state_file" 2>/dev/null || true)"
      if [ -n "$VM_ID" ] && command -v vmsan >/dev/null 2>&1; then
        info "Stopping VM $VM_ID..."
        vmsan stop "$VM_ID" 2>/dev/null || true
      fi
      [ -n "$VM_PID" ] && kill -9 "$VM_PID" 2>/dev/null || true
    done
  fi

  if [ -f "$VMSAN_DIR/cloudflare/cloudflared.pid" ]; then
    CF_PID="$(cat "$VMSAN_DIR/cloudflare/cloudflared.pid" 2>/dev/null || true)"
    if [ -n "${CF_PID:-}" ] && [ -d "/proc/$CF_PID" ]; then
      CF_COMM="$(cat "/proc/$CF_PID/comm" 2>/dev/null || true)"
      if [ "$CF_COMM" = "cloudflared" ]; then
        kill -TERM "$CF_PID" 2>/dev/null || true
      fi
    fi
  fi

  if command -v vmsan >/dev/null 2>&1; then
    info "Removing vmsan CLI (npm global)..."
    npm uninstall -g vmsan
    success "vmsan CLI removed"
  else
    success "vmsan CLI not installed"
  fi

  # Clean up TAP and veth interfaces (host namespace)
  NET_COUNT=0
  for iface_path in /sys/class/net/fhvm* /sys/class/net/veth-h-* /sys/class/net/veth-g-*; do
    [ -e "$iface_path" ] || continue
    DEV="$(basename "$iface_path")"
    ip link delete "$DEV" 2>/dev/null && NET_COUNT=$((NET_COUNT + 1))
  done

  # Clean up network namespaces created by vmsan
  for ns in $(ip netns list 2>/dev/null | awk '{print $1}' | grep '^vmsan-'); do
    ip netns delete "$ns" 2>/dev/null && NET_COUNT=$((NET_COUNT + 1))
  done

  [ "$NET_COUNT" -gt 0 ] && success "Cleaned up $NET_COUNT network resources"

  # Clean up iptables rules referencing vmsan
  iptables-save 2>/dev/null | grep -cE 'fhvm|172\.16\.' >/dev/null 2>&1 && {
    info "Cleaning iptables rules..."
    iptables -t nat -S 2>/dev/null | grep -E 'fhvm|172\.16\.' | while IFS= read -r rule; do
      eval "iptables -t nat $(echo "$rule" | sed 's/^-A/-D/')" 2>/dev/null || true
    done
    iptables -S FORWARD 2>/dev/null | grep -E 'fhvm|172\.16\.' | while IFS= read -r rule; do
      eval "iptables $(echo "$rule" | sed 's/^-A/-D/')" 2>/dev/null || true
    done
    success "iptables rules cleaned"
  }

  # Delete Cloudflare tunnel via API before removing local files
  if [ -f "$VMSAN_DIR/cloudflare/cloudflare.json" ]; then
    CF_TOKEN="$(grep -oP '"token"\s*:\s*"\K[^"]+' "$VMSAN_DIR/cloudflare/cloudflare.json" 2>/dev/null || true)"
    CF_TUNNEL_ID="$(grep -oP '"tunnelId"\s*:\s*"\K[^"]+' "$VMSAN_DIR/cloudflare/cloudflare.json" 2>/dev/null || true)"
    CF_ACCOUNT_ID="$(grep -oP '"accountId"\s*:\s*"\K[^"]+' "$VMSAN_DIR/cloudflare/cloudflare.json" 2>/dev/null || true)"
    if [ -n "$CF_TOKEN" ] && [ -n "$CF_TUNNEL_ID" ] && [ -n "$CF_ACCOUNT_ID" ]; then
      info "Deleting Cloudflare Tunnel $CF_TUNNEL_ID..."
      # Clean tunnel connections first, then delete
      curl -fsSL -X DELETE \
        -H "Authorization: Bearer $CF_TOKEN" \
        -H "Content-Type: application/json" \
        "https://api.cloudflare.com/client/v4/accounts/$CF_ACCOUNT_ID/cfd_tunnel/$CF_TUNNEL_ID/connections" >/dev/null 2>&1 || true
      if curl -fsSL -o /dev/null -X DELETE \
        -H "Authorization: Bearer $CF_TOKEN" \
        -H "Content-Type: application/json" \
        "https://api.cloudflare.com/client/v4/accounts/$CF_ACCOUNT_ID/cfd_tunnel/$CF_TUNNEL_ID" 2>/dev/null; then
        success "Cloudflare Tunnel deleted"
      else
        warn "Could not delete Cloudflare Tunnel (may need manual cleanup)"
      fi
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
      read -rs CF_TOKEN </dev/tty
      echo ""

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

# --- runtime images ---

build_runtime() {
  local name="$1"
  local base_image="$2"
  local dest="$VMSAN_DIR/rootfs/${name}.ext4"

  if [ -f "$dest" ]; then
    success "Runtime ${name} already built"
    return
  fi

  need_cmd docker

  info "Building runtime ${name} from ${base_image}..."

  local build_dir
  build_dir=$(mktemp -d)
  local build_tag="vmsan-rootfs-${name}:latest"
  local container_name="vmsan-export-${name}-$$"
  trap 'docker rm -f "$container_name" >/dev/null 2>&1 || true; rm -rf "$build_dir"' RETURN

  # Detect package manager and install appropriate packages
  cat > "$build_dir/Dockerfile" <<'DEOF'
FROM __BASE_IMAGE__
RUN if command -v apt-get >/dev/null 2>&1; then \
      apt-get update && apt-get install -y --no-install-recommends \
        bind9-utils bzip2 findutils git gzip iputils-ping libicu-dev libjpeg-dev \
        libpng-dev ncurses-base libssl-dev openssh-server openssl procps sudo \
        systemd systemd-sysv tar unzip debianutils whois zstd \
      && rm -rf /var/lib/apt/lists/*; \
    elif command -v dnf >/dev/null 2>&1; then \
      dnf install -y bind-utils bzip2 findutils git gzip iputils libicu libjpeg \
        libpng ncurses-libs openssh-server openssl openssl-libs procps sudo \
        systemd tar unzip which whois zstd \
      && dnf clean all; \
    elif command -v apk >/dev/null 2>&1; then \
      apk add --no-cache bash bind-tools bzip2 findutils git gzip iputils \
        icu-libs libjpeg-turbo libpng ncurses-libs openrc openssh openssl \
        procps sudo tar unzip whois zstd; \
    fi
RUN if command -v apk >/dev/null 2>&1; then \
      id -u ubuntu >/dev/null 2>&1 || adduser -D -s /bin/bash ubuntu; \
    else \
      id -u ubuntu >/dev/null 2>&1 || useradd -m -s /bin/bash ubuntu; \
    fi; \
    echo 'ubuntu ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/ubuntu; \
    chmod 440 /etc/sudoers.d/ubuntu; \
    mkdir -p /home/ubuntu/.ssh && chown -R ubuntu:ubuntu /home/ubuntu
RUN ssh-keygen -A 2>/dev/null || true; \
    mkdir -p /root/.ssh && chmod 700 /root/.ssh; \
    if [ -f /etc/ssh/sshd_config ]; then \
      sed -i 's/^#*PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config; \
    fi; \
    if command -v rc-update >/dev/null 2>&1; then \
      rc-update add devfs sysinit 2>/dev/null || true; \
      rc-update add mdev sysinit 2>/dev/null || true; \
      rc-update add hwdrivers sysinit 2>/dev/null || true; \
      rc-update add modules boot 2>/dev/null || true; \
      rc-update add sysctl boot 2>/dev/null || true; \
      rc-update add hostname boot 2>/dev/null || true; \
      rc-update add bootmisc boot 2>/dev/null || true; \
      rc-update add networking boot 2>/dev/null || true; \
      rc-update add sshd default 2>/dev/null || true; \
      printf '%s\n' '::sysinit:/sbin/openrc sysinit' '::sysinit:/sbin/openrc boot' '::wait:/sbin/openrc default' '::shutdown:/sbin/openrc shutdown' 'ttyS0::respawn:/sbin/getty 115200 ttyS0' > /etc/inittab; \
    fi; \
    if command -v systemctl >/dev/null 2>&1; then systemctl enable sshd 2>/dev/null || systemctl enable ssh 2>/dev/null || true; fi
DEOF

  sed -i "s|__BASE_IMAGE__|${base_image}|" "$build_dir/Dockerfile"

  # Build
  docker build -t "$build_tag" -f "$build_dir/Dockerfile" "$build_dir" >/dev/null 2>&1

  # Export
  docker create --name "$container_name" "$build_tag" >/dev/null 2>&1
  docker export "$container_name" -o "$build_dir/rootfs.tar" 2>/dev/null

  # Convert to ext4
  local tar_bytes
  tar_bytes=$(stat -c %s "$build_dir/rootfs.tar")
  local tar_mb=$(( tar_bytes / 1024 / 1024 ))
  local image_mb=$(( tar_mb + 512 ))
  [ "$image_mb" -lt 1024 ] && image_mb=1024

  dd if=/dev/zero of="$dest" bs=1M count="$image_mb" status=none
  mkfs.ext4 -q "$dest"
  tune2fs -m 0 "$dest" >/dev/null 2>&1

  local mnt_dir="$build_dir/mnt"
  mkdir -p "$mnt_dir"
  mount -o loop "$dest" "$mnt_dir"
  tar -xf "$build_dir/rootfs.tar" -C "$mnt_dir"
  umount "$mnt_dir"

  success "Runtime ${name} built (${image_mb} MB)"
}

if command -v docker >/dev/null 2>&1; then
  build_runtime "node22" "node:22"
  build_runtime "node24" "node:24"
  build_runtime "python3.13" "python:3.13-slim"
else
  warn "Docker not found — skipping runtime image builds. Install Docker and re-run to build runtime images."
fi

# --- summary ---

CF_STATUS="not configured"
[ -f "$VMSAN_DIR/cloudflare/cloudflare.json" ] && CF_STATUS="configured"

RUNTIME_STATUS=""
for rt in node22 node24 python3.13; do
  if [ -f "$VMSAN_DIR/rootfs/${rt}.ext4" ]; then
    RUNTIME_STATUS="${RUNTIME_STATUS}${rt} "
  fi
done
RUNTIME_STATUS="${RUNTIME_STATUS:-none}"

echo ""
success "vmsan environment ready at $VMSAN_DIR"
echo ""
echo "  Firecracker  $VMSAN_DIR/bin/firecracker"
echo "  Jailer       $VMSAN_DIR/bin/jailer"
echo "  Kernel       $VMSAN_DIR/kernels/$KERNEL_FILE"
echo "  Rootfs       $VMSAN_DIR/rootfs/$ROOTFS_FILE"
echo "  Runtimes     $RUNTIME_STATUS"
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
