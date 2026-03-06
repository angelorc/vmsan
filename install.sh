#!/usr/bin/env bash
set -euo pipefail

# vmsan installer — downloads Firecracker, kernel, rootfs, and vmsan-agent.
# Usage:
#   Install: curl -fsSL https://vmsan.dev/install | bash
#   Install a branch: curl -fsSL https://vmsan.dev/install | bash -s -- --ref my-branch
#   Install a commit: curl -fsSL https://vmsan.dev/install | bash -s -- --sha <commit>
#   Uninstall: curl -fsSL https://vmsan.dev/install | bash -s -- --uninstall

VMSAN_DIR="${VMSAN_DIR:-}"
VMSAN_REPO="angelorc/vmsan"
VMSAN_INSTALL_BOOTSTRAPPED="${VMSAN_INSTALL_BOOTSTRAPPED:-}"
VMSAN_INSTALL_SOURCE_SHA="${VMSAN_INSTALL_SOURCE_SHA:-}"
VMSAN_INSTALL_REQUESTED_REF="${VMSAN_INSTALL_REQUESTED_REF:-}"
VMSAN_INSTALL_REQUESTED_SHA="${VMSAN_INSTALL_REQUESTED_SHA:-}"
ARCH="$(uname -m)"
CLOUDFLARED_VERSION="${CLOUDFLARED_VERSION:-2026.2.0}"
GO_REQUIRED_VERSION="${GO_REQUIRED_VERSION:-1.22.0}"
GO_INSTALL_VERSION="${GO_INSTALL_VERSION:-1.22.0}"
REQUESTED_REF="$VMSAN_INSTALL_REQUESTED_REF"
REQUESTED_SHA="$VMSAN_INSTALL_REQUESTED_SHA"
UNINSTALL=0

# --- helpers ---

info()    { printf "\033[1;34m[info]\033[0m  %s\n" "$*"; }
success() { printf "\033[1;32m[ok]\033[0m    %s\n" "$*"; }
warn()    { printf "\033[1;33m[warn]\033[0m  %s\n" "$*"; }
error()   { printf "\033[1;31m[error]\033[0m %s\n" "$*" >&2; exit 1; }

resolve_vmsan_dir() {
  if [ -n "$VMSAN_DIR" ]; then
    printf '%s\n' "$VMSAN_DIR"
    return
  fi

  if [ -n "${SUDO_USER:-}" ] && command -v getent >/dev/null 2>&1; then
    local sudo_home
    sudo_home="$(getent passwd "$SUDO_USER" 2>/dev/null | cut -d: -f6)"
    if [ -n "$sudo_home" ]; then
      printf '%s/.vmsan\n' "$sudo_home"
      return
    fi
  fi

  printf '%s/.vmsan\n' "$HOME"
}

print_usage() {
  cat <<EOF
Usage:
  curl -fsSL https://vmsan.dev/install | bash
  curl -fsSL https://vmsan.dev/install | bash -s -- --ref <branch>
  curl -fsSL https://vmsan.dev/install | bash -s -- --sha <commit>
  curl -fsSL https://vmsan.dev/install | bash -s -- --uninstall
EOF
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || error "Required command not found: $1"
}

default_iface() {
  ip route show default 2>/dev/null | awk '/default/ {print $5; exit}'
}

json_string_field() {
  local key="$1" file="$2"
  grep -oP "\"$key\"\\s*:\\s*\"\\K[^\"]+" "$file" 2>/dev/null | head -n1 || true
}

json_bool_field() {
  local key="$1" file="$2"
  grep -oP "\"$key\"\\s*:\\s*\\K(true|false)" "$file" 2>/dev/null | head -n1 || true
}

json_ports_field() {
  local file="$1"
  sed -n '/"publishedPorts"\s*:/,/\]/p' "$file" 2>/dev/null | grep -oP '\d+' || true
}

cidr30_from_ip() {
  local ip="$1"
  IFS=. read -r a b c _ <<EOF
$ip
EOF
  [ -n "${a:-}" ] && [ -n "${b:-}" ] && [ -n "${c:-}" ] || return 1
  printf '%s.%s.%s.0/30\n' "$a" "$b" "$c"
}

iptables_delete_rule() {
  iptables "$@" 2>/dev/null || true
}

cleanup_iptables_rules_for_vm() {
  local state_file="$1" default_iface_name="$2"
  local tap_device guest_ip skip_dnat slot veth_host guest_cidr

  tap_device="$(json_string_field tapDevice "$state_file")"
  guest_ip="$(json_string_field guestIp "$state_file")"
  skip_dnat="$(json_bool_field skipDnat "$state_file")"

  [ -n "$guest_ip" ] || return
  guest_cidr="$(cidr30_from_ip "$guest_ip" || true)"

  if [ -n "$default_iface_name" ] && [ -n "$guest_cidr" ]; then
    iptables_delete_rule -t nat -D POSTROUTING -s "$guest_cidr" -o "$default_iface_name" -j MASQUERADE

    if [ "$skip_dnat" != "true" ]; then
      while IFS= read -r port; do
        [ -n "$port" ] || continue
        iptables_delete_rule -t nat -D PREROUTING -i "$default_iface_name" -p tcp --dport "$port" -j DNAT --to-destination "${guest_ip}:${port}"
        iptables_delete_rule -D FORWARD -p tcp -d "$guest_ip" --dport "$port" -j ACCEPT
      done < <(json_ports_field "$state_file")
    fi
  fi

  slot="$(printf '%s\n' "$tap_device" | sed -n 's/^fhvm//p')"
  if [ -n "$default_iface_name" ] && [ -n "$slot" ] && [ -n "$guest_cidr" ]; then
    veth_host="veth-h-$slot"
    iptables_delete_rule -D FORWARD -i "$veth_host" -o "$default_iface_name" -s "$guest_cidr" -j ACCEPT
    iptables_delete_rule -D FORWARD -i "$default_iface_name" -o "$veth_host" -d "$guest_cidr" -m state --state RELATED,ESTABLISHED -j ACCEPT
  fi

  if [ -n "$tap_device" ]; then
    iptables -S 2>/dev/null | grep -E "(^-A .* -i ${tap_device}( |$)|^-A .* -o ${tap_device}( |$))" | while IFS= read -r rule; do
      eval "iptables $(echo "$rule" | sed 's/^-A/-D/')" 2>/dev/null || true
    done || true
  fi

  if [ -n "$slot" ]; then
    veth_host="veth-h-$slot"
    iptables -S 2>/dev/null | grep -E "(^-A .* -i ${veth_host}( |$)|^-A .* -o ${veth_host}( |$))" | while IFS= read -r rule; do
      eval "iptables $(echo "$rule" | sed 's/^-A/-D/')" 2>/dev/null || true
    done || true
  fi
}

VMSAN_DIR="$(resolve_vmsan_dir)"

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

version_ge() {
  local current="$1"
  local minimum="$2"
  [ "$(printf '%s\n%s\n' "$minimum" "$current" | sort -V | head -n1)" = "$minimum" ]
}

go_version_number() {
  go version 2>/dev/null | awk '{print $3}' | sed 's/^go//'
}

resolve_commit_sha() {
  local target="$1"
  local sha
  sha="$(curl -fsSL -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/$VMSAN_REPO/commits/$target" \
    | grep -oP '"sha":\s*"\K[0-9a-f]{40}' | head -1)"
  [ -n "$sha" ] || error "Could not resolve commit for $target"
  printf '%s\n' "$sha"
}

bootstrap_ref_installer() {
  local resolved_sha="$1"
  shift
  local script_tmp
  script_tmp="$(mktemp)"

  download "https://raw.githubusercontent.com/$VMSAN_REPO/$resolved_sha/install.sh" "$script_tmp"
  chmod +x "$script_tmp"

  info "Re-executing installer from commit $resolved_sha..."
  local -a env_args=(
    "VMSAN_INSTALL_BOOTSTRAPPED=1"
    "VMSAN_INSTALL_SOURCE_SHA=$resolved_sha"
    "VMSAN_INSTALL_REQUESTED_REF=$REQUESTED_REF"
    "VMSAN_INSTALL_REQUESTED_SHA=$REQUESTED_SHA"
  )
  local status
  if env "${env_args[@]}" bash "$script_tmp" "$@"; then
    status=0
  else
    status=$?
  fi
  rm -f -- "$script_tmp"
  return "$status"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --uninstall)
      UNINSTALL=1
      shift
      ;;
    --ref)
      [ $# -ge 2 ] || error "--ref requires a value"
      REQUESTED_REF="$2"
      shift 2
      ;;
    --ref=*)
      REQUESTED_REF="${1#--ref=}"
      shift
      ;;
    --sha)
      [ $# -ge 2 ] || error "--sha requires a value"
      REQUESTED_SHA="$2"
      shift 2
      ;;
    --sha=*)
      REQUESTED_SHA="${1#--sha=}"
      shift
      ;;
    --help|-h)
      print_usage
      exit 0
      ;;
    *)
      error "Unknown argument: $1"
      ;;
  esac
done

[ -n "$REQUESTED_REF" ] && [ -n "$REQUESTED_SHA" ] && error "Use only one of --ref or --sha"

if [ -z "$VMSAN_INSTALL_BOOTSTRAPPED" ] && { [ -n "$REQUESTED_REF" ] || [ -n "$REQUESTED_SHA" ]; }; then
  need_cmd curl
  INSTALL_TARGET="${REQUESTED_SHA:-$REQUESTED_REF}"
  RESOLVED_BOOTSTRAP_SHA="$(resolve_commit_sha "$INSTALL_TARGET")"
  FORWARD_ARGS=()
  [ "$UNINSTALL" -eq 1 ] && FORWARD_ARGS+=(--uninstall)
  bootstrap_ref_installer "$RESOLVED_BOOTSTRAP_SHA" "${FORWARD_ARGS[@]}"
  exit 0
fi

INSTALL_MODE="release"
SOURCE_LABEL=""
SOURCE_SHA=""
if [ -n "$VMSAN_INSTALL_SOURCE_SHA" ]; then
  INSTALL_MODE="source"
  SOURCE_SHA="$VMSAN_INSTALL_SOURCE_SHA"
  SOURCE_LABEL="${REQUESTED_REF:-${REQUESTED_SHA:-$SOURCE_SHA}}"
fi

# --- uninstall ---

if [ "$UNINSTALL" -eq 1 ]; then
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

  # Clean up iptables rules owned by vmsan using per-VM state instead of broad 172.16.* matching.
  DEFAULT_IFACE="$(default_iface || true)"
  IPTABLES_CLEANED=0
  if [ -d "$VMSAN_DIR/vms" ]; then
    for state_file in "$VMSAN_DIR"/vms/*.json; do
      [ -f "$state_file" ] || continue
      [ "$IPTABLES_CLEANED" -eq 0 ] && info "Cleaning vmsan iptables rules..."
      cleanup_iptables_rules_for_vm "$state_file" "$DEFAULT_IFACE"
      IPTABLES_CLEANED=1
    done
  fi
  [ "$IPTABLES_CLEANED" -eq 1 ] && success "iptables rules cleaned"

  # Clean up TAP and veth interfaces (host namespace)
  NET_COUNT=0
  for iface_path in /sys/class/net/fhvm* /sys/class/net/veth-h-* /sys/class/net/veth-g-*; do
    [ -e "$iface_path" ] || continue
    DEV="$(basename "$iface_path")"
    ip link delete "$DEV" 2>/dev/null && NET_COUNT=$((NET_COUNT + 1))
  done

  # Clean up network namespaces created by vmsan
  for ns in $(ip netns list 2>/dev/null | awk '{print $1}' | grep '^vmsan-' || true); do
    ip netns delete "$ns" 2>/dev/null && NET_COUNT=$((NET_COUNT + 1))
  done

  [ "$NET_COUNT" -gt 0 ] && success "Cleaned up $NET_COUNT network resources"

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

ensure_go() {
  local current_go_version=""

  if command -v go >/dev/null 2>&1; then
    current_go_version="$(go_version_number)"
    if [ -n "$current_go_version" ] && version_ge "$current_go_version" "$GO_REQUIRED_VERSION"; then
      return
    fi
    warn "Go ${current_go_version:-unknown} is too old; installing Go ${GO_INSTALL_VERSION}..."
  else
    info "Go not found — installing Go ${GO_INSTALL_VERSION}..."
  fi

  local go_tmp
  go_tmp="$(mktemp -d)"
  trap 'rm -rf "$go_tmp"' RETURN

  local go_tarball="go${GO_INSTALL_VERSION}.linux-$(go_arch).tar.gz"
  download "https://go.dev/dl/${go_tarball}" "$go_tmp/$go_tarball"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "$go_tmp/$go_tarball"
  ln -sf /usr/local/go/bin/go /usr/local/bin/go
  ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
  hash -r

  current_go_version="$(go_version_number)"
  [ -n "$current_go_version" ] || error "Go installation failed"
  version_ge "$current_go_version" "$GO_REQUIRED_VERSION" \
    || error "Installed Go $current_go_version is still below required version $GO_REQUIRED_VERSION"

  trap - RETURN
  rm -rf "$go_tmp"
  success "Go $(go version | awk '{print $3}') installed"
}

ensure_node() {
  if command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1; then
    return
  fi

  if command -v apt-get >/dev/null 2>&1; then
    info "Node.js not found — installing Node.js 22 via NodeSource..."
    curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
    apt-get install -y -qq nodejs >/dev/null 2>&1
  elif command -v dnf >/dev/null 2>&1 || command -v yum >/dev/null 2>&1; then
    info "Node.js/npm not found — installing nodejs via the system package manager..."
    install_pkg nodejs
  else
    error "Node.js and npm are required, but no supported package manager is available."
  fi

  command -v node >/dev/null 2>&1 || error "Node.js installation failed"
  command -v npm >/dev/null 2>&1 || error "npm is required but was not installed"
  success "Node.js $(node --version) installed"
}

download_source_tree() {
  local sha="$1"
  local dest="$2"

  rm -rf "$dest"
  mkdir -p "$dest"
  info "Fetching vmsan source at $sha..."
  curl -fsSL "https://github.com/$VMSAN_REPO/archive/${sha}.tar.gz" \
    | tar -xz --strip-components=1 -C "$dest"
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

ensure_node

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

# --- source / release selection ---

VMSAN_SRC="$VMSAN_DIR/src"
LATEST_TAG=""

if [ "$INSTALL_MODE" = "source" ]; then
  info "Source install mode active (${SOURCE_LABEL})"
  download_source_tree "$SOURCE_SHA" "$VMSAN_SRC"
else
  info "Fetching latest release tag..."
  LATEST_TAG=$(curl -fsSL "https://api.github.com/repos/$VMSAN_REPO/releases" | grep -oP '"tag_name":\s*"\K[^"]+' | head -1)
  [ -n "$LATEST_TAG" ] || error "Could not determine latest release tag"
  success "Latest release: $LATEST_TAG"
fi

# --- vmsan CLI ---

if [ "$INSTALL_MODE" = "source" ]; then
  info "Installing vmsan CLI from source (${SOURCE_SHA})..."
  (cd "$VMSAN_SRC" && npm install --ignore-scripts && npx obuild)
  npm install -g "$VMSAN_SRC"
  VMSAN_VER=$(vmsan --version 2>/dev/null || echo "unknown")
  success "vmsan CLI installed from ${SOURCE_LABEL} ($VMSAN_VER)"
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

if [ "$INSTALL_MODE" = "source" ]; then
  ensure_go
  info "Building vmsan-agent from source (${SOURCE_SHA})..."
  (cd "$VMSAN_SRC/agent" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go_arch)" go build -ldflags="-s -w" -o "$AGENT_PATH" .)
  chmod +x "$AGENT_PATH"
  success "vmsan-agent built from ${SOURCE_LABEL}"
elif [ -x "$AGENT_PATH" ]; then
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
    echo "  │  Cloudflare Tunnel (optional)                               │"
    echo "  │                                                             │"
    echo "  │  vmsan can expose VMs via Cloudflare Tunnels.               │"
    echo "  │  You need a Cloudflare API token and a domain managed       │"
    echo "  │  by Cloudflare.                                             │"
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

RUNTIME_RECIPE_VERSION="2"

runtime_metadata_path() {
  local name="$1"
  echo "$VMSAN_DIR/rootfs/${name}.meta"
}

runtime_metadata_matches() {
  local name="$1"
  local base_image="$2"
  local meta
  meta="$(runtime_metadata_path "$name")"

  [ -f "$meta" ] || return 1

  local recipe_version
  local recorded_base_image
  recipe_version="$(sed -n 's/^recipe_version=//p' "$meta" | head -n1)"
  recorded_base_image="$(sed -n 's/^base_image=//p' "$meta" | head -n1)"

  [ "$recipe_version" = "$RUNTIME_RECIPE_VERSION" ] && [ "$recorded_base_image" = "$base_image" ]
}

write_runtime_metadata() {
  local name="$1"
  local base_image="$2"
  local meta
  meta="$(runtime_metadata_path "$name")"

  cat > "$meta" <<EOF
recipe_version=$RUNTIME_RECIPE_VERSION
base_image=$base_image
built_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF
}

build_runtime() {
  local name="$1"
  local base_image="$2"
  local dest="$VMSAN_DIR/rootfs/${name}.ext4"
  local meta
  meta="$(runtime_metadata_path "$name")"

  if [ -f "$dest" ] && runtime_metadata_matches "$name" "$base_image"; then
    success "Runtime ${name} already built"
    return
  fi

  if [ -f "$dest" ]; then
    info "Rebuilding runtime ${name} to refresh the runtime recipe..."
    rm -f "$dest" "$meta"
  fi

  need_cmd docker

  info "Building runtime ${name} from ${base_image}..."

  local build_dir
  build_dir=$(mktemp -d)
  local mnt_dir="$build_dir/mnt"
  local build_tag="vmsan-rootfs-${name}:latest"
  local container_name="vmsan-export-${name}-$$"
  trap 'trap - RETURN; grep -qs " $mnt_dir " /proc/mounts && umount "$mnt_dir" >/dev/null 2>&1 || true; docker rm -f "$container_name" >/dev/null 2>&1 || true; rm -rf "$build_dir"' RETURN

  # Detect package manager and install appropriate packages
  cat > "$build_dir/Dockerfile" <<'DEOF'
FROM __BASE_IMAGE__
RUN if command -v apt-get >/dev/null 2>&1; then \
      apt-get update && apt-get install -y --no-install-recommends \
        bind9-utils bzip2 findutils git gzip iptables iputils-ping libicu-dev libjpeg-dev \
        libpng-dev ncurses-base libssl-dev openssl procps sudo \
        systemd systemd-sysv tar unzip debianutils whois zstd \
      && rm -rf /var/lib/apt/lists/*; \
    elif command -v dnf >/dev/null 2>&1; then \
      dnf install -y bind-utils bzip2 findutils git gzip iptables iputils libicu libjpeg \
        libpng ncurses-libs openssl openssl-libs procps sudo \
        systemd tar unzip which whois zstd \
      && dnf clean all; \
    elif command -v apk >/dev/null 2>&1; then \
      apk add --no-cache bash bind-tools bzip2 findutils git gzip iptables iputils \
        icu-libs libjpeg-turbo libpng ncurses-libs openrc openssl \
        procps sudo tar unzip whois zstd; \
    fi
RUN if command -v apk >/dev/null 2>&1; then \
      id -u ubuntu >/dev/null 2>&1 || adduser -D -s /bin/bash ubuntu; \
    else \
      id -u ubuntu >/dev/null 2>&1 || useradd -m -s /bin/bash ubuntu; \
    fi; \
    echo 'ubuntu ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/ubuntu; \
    chmod 440 /etc/sudoers.d/ubuntu; \
    chown -R ubuntu:ubuntu /home/ubuntu
RUN EXTRA=""; \
    if command -v npm >/dev/null 2>&1; then \
      su -c 'mkdir -p /home/ubuntu/.npm-global && npm config set prefix /home/ubuntu/.npm-global' ubuntu; \
      EXTRA="${EXTRA}export PATH=\"/home/ubuntu/.npm-global/bin:\$PATH\"\n"; \
    fi; \
    if command -v pip3 >/dev/null 2>&1 || command -v pip >/dev/null 2>&1; then \
      EXTRA="${EXTRA}export PATH=\"/home/ubuntu/.local/bin:\$PATH\"\n"; \
    fi; \
    if [ -n "$EXTRA" ]; then \
      printf '%b' "$EXTRA" >> /home/ubuntu/.profile; \
      { printf '%b' "$EXTRA"; cat /home/ubuntu/.bashrc; } > /home/ubuntu/.bashrc.tmp \
        && mv /home/ubuntu/.bashrc.tmp /home/ubuntu/.bashrc; \
    fi; \
    chown -R ubuntu:ubuntu /home/ubuntu
RUN if command -v rc-update >/dev/null 2>&1; then \
      rc-update add devfs sysinit 2>/dev/null || true; \
      rc-update add mdev sysinit 2>/dev/null || true; \
      rc-update add hwdrivers sysinit 2>/dev/null || true; \
      rc-update add modules boot 2>/dev/null || true; \
      rc-update add sysctl boot 2>/dev/null || true; \
      rc-update add hostname boot 2>/dev/null || true; \
      rc-update add bootmisc boot 2>/dev/null || true; \
      rc-update add networking boot 2>/dev/null || true; \
      printf '%s\n' '::sysinit:/sbin/openrc sysinit' '::sysinit:/sbin/openrc boot' '::wait:/sbin/openrc default' '::shutdown:/sbin/openrc shutdown' 'ttyS0::respawn:/sbin/getty 115200 ttyS0' > /etc/inittab; \
    fi
DEOF

  sed -i "s|__BASE_IMAGE__|${base_image}|" "$build_dir/Dockerfile"

  # Build
  docker build -t "$build_tag" -f "$build_dir/Dockerfile" "$build_dir" >/dev/null \
    || error "Failed to build runtime ${name} from ${base_image}"

  # Export
  docker create --name "$container_name" "$build_tag" >/dev/null \
    || error "Failed to create export container for runtime ${name}"
  docker export "$container_name" -o "$build_dir/rootfs.tar" \
    || error "Failed to export rootfs tar for runtime ${name}"

  # Convert to ext4
  local tar_bytes
  tar_bytes=$(stat -c %s "$build_dir/rootfs.tar")
  local tar_mb=$(( tar_bytes / 1024 / 1024 ))
  local image_mb=$(( tar_mb + 512 ))
  [ "$image_mb" -lt 1024 ] && image_mb=1024

  dd if=/dev/zero of="$dest" bs=1M count="$image_mb" status=none
  mkfs.ext4 -q "$dest"
  tune2fs -m 0 "$dest" >/dev/null 2>&1

  mkdir -p "$mnt_dir"
  mount -o loop "$dest" "$mnt_dir"
  tar -xf "$build_dir/rootfs.tar" -C "$mnt_dir"
  umount "$mnt_dir"

  write_runtime_metadata "$name" "$base_image"

  success "Runtime ${name} built (${image_mb} MB)"
}

if command -v docker >/dev/null 2>&1; then
  build_runtime "node22" "node:22"
  build_runtime "node24" "node:24"
  build_runtime "python3.13" "python:3.13-slim"
else
  warn "Docker not found — skipping runtime image builds. Install Docker and re-run to build runtime images."
fi

# --- install metadata ---

INSTALL_METADATA="$VMSAN_DIR/install.meta"
cat > "$INSTALL_METADATA" <<EOF
mode=$INSTALL_MODE
requested_ref=$REQUESTED_REF
requested_sha=$REQUESTED_SHA
resolved_sha=$SOURCE_SHA
source_label=$SOURCE_LABEL
latest_release=$LATEST_TAG
cli_version=$VMSAN_VER
runtime_recipe_version=$RUNTIME_RECIPE_VERSION
installed_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF

# --- summary ---

CF_STATUS="not configured"
[ -f "$VMSAN_DIR/cloudflare/cloudflare.json" ] && CF_STATUS="configured"

RUNTIME_STATUS=""
for rt in node22 node24 python3.13; do
  if [ -f "$VMSAN_DIR/rootfs/${rt}.ext4" ]; then
    RUNTIME_STATUS="${RUNTIME_STATUS}${rt} "
  fi
done
RUNTIME_STATUS="${RUNTIME_STATUS% }"
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
if [ "$INSTALL_MODE" = "source" ]; then
  echo "  Source       $SOURCE_SHA ($SOURCE_LABEL)"
else
  echo "  Release      ${LATEST_TAG:-unknown}"
fi
echo "  Metadata     $INSTALL_METADATA"
echo ""
if [ "$CF_STATUS" = "configured" ]; then
  echo "  Tunnel mode active — VMs exposed via Cloudflare (no DNAT)."
  echo ""
elif [ "$CF_STATUS" = "not configured" ]; then
  echo "  To configure Cloudflare later, re-run this installer."
  echo ""
fi
