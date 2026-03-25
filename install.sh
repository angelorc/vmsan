#!/usr/bin/env bash
set -euo pipefail

# vmsan installer — downloads Firecracker, kernel, rootfs, and vmsan-agent.
# Usage:
#   Install: curl -fsSL https://vmsan.dev/install | bash
#   Install from local checkout: sudo bash ./install.sh --local
#   Install from local checkout: sudo bash ./install.sh --source-dir /path/to/vmsan
#   Install a branch: curl -fsSL https://vmsan.dev/install | bash -s -- --ref my-branch
#   Install a commit: curl -fsSL https://vmsan.dev/install | bash -s -- --sha <commit>
#   Uninstall: curl -fsSL https://vmsan.dev/install | bash -s -- --uninstall

VMSAN_DIR="${VMSAN_DIR:-}"
VMSAN_REPO="angelorc/vmsan"
VMSAN_INSTALL_BOOTSTRAPPED="${VMSAN_INSTALL_BOOTSTRAPPED:-}"
VMSAN_INSTALL_SOURCE_SHA="${VMSAN_INSTALL_SOURCE_SHA:-}"
VMSAN_INSTALL_SOURCE_DIR="${VMSAN_INSTALL_SOURCE_DIR:-}"
VMSAN_INSTALL_REQUESTED_REF="${VMSAN_INSTALL_REQUESTED_REF:-}"
VMSAN_INSTALL_REQUESTED_SHA="${VMSAN_INSTALL_REQUESTED_SHA:-}"
VMSAN_RUNTIME_MANIFEST_URL="${VMSAN_RUNTIME_MANIFEST_URL:-https://artifacts.vmsan.dev/runtimes/channels/stable.json}"
VMSAN_FORCE_LOCAL_RUNTIME_BUILD="${VMSAN_FORCE_LOCAL_RUNTIME_BUILD:-0}"
ARCH="$(uname -m)"
CLOUDFLARED_VERSION="${CLOUDFLARED_VERSION:-2026.2.0}"
GO_REQUIRED_VERSION="${GO_REQUIRED_VERSION:-1.22.0}"
GO_INSTALL_VERSION="${GO_INSTALL_VERSION:-1.22.0}"
REQUESTED_REF="$VMSAN_INSTALL_REQUESTED_REF"
REQUESTED_SHA="$VMSAN_INSTALL_REQUESTED_SHA"
REQUESTED_SOURCE_DIR="$VMSAN_INSTALL_SOURCE_DIR"
RUNTIME_MANIFEST_URL="$VMSAN_RUNTIME_MANIFEST_URL"
FORCE_LOCAL_RUNTIME_BUILD="$VMSAN_FORCE_LOCAL_RUNTIME_BUILD"
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
  sudo bash ./install.sh --local
  sudo bash ./install.sh --source-dir /path/to/vmsan
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

json_number_field() {
  local key="$1" file="$2"
  grep -oP "\"$key\"\\s*:\\s*\\K[0-9]+" "$file" 2>/dev/null | head -n1 || true
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

delete_iptables_rule_spec() {
  local rule="$1"
  local -a rule_parts
  read -r -a rule_parts <<<"$rule"
  [ "${#rule_parts[@]}" -gt 0 ] || return 0
  [ "${rule_parts[0]}" = "-A" ] || return 0
  rule_parts[0]="-D"
  iptables_delete_rule "${rule_parts[@]}"
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
    iptables_delete_rule -D OUTPUT -d "$guest_ip" -j ACCEPT
    iptables_delete_rule -D INPUT -s "$guest_ip" -j ACCEPT

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
      delete_iptables_rule_spec "$rule"
    done || true
  fi

  if [ -n "$slot" ]; then
    veth_host="veth-h-$slot"
    iptables -S 2>/dev/null | grep -E "(^-A .* -i ${veth_host}( |$)|^-A .* -o ${veth_host}( |$))" | while IFS= read -r rule; do
      delete_iptables_rule_spec "$rule"
    done || true
  fi
}

VMSAN_DIR="$(resolve_vmsan_dir)"

download() {
  local url="$1" dest="$2"
  local tmp_dest
  tmp_dest="$(mktemp)"
  info "Downloading $(basename "$dest")..."
  if ! curl -fsSL -o "$tmp_dest" "$url"; then
    rm -f "$tmp_dest"
    error "Failed to download $(basename "$dest") from $url"
  fi
  mv "$tmp_dest" "$dest"
}

go_arch() {
  case "$ARCH" in
    x86_64)  echo "amd64" ;;
    aarch64) echo "arm64" ;;
    *)       error "Unsupported architecture: $ARCH" ;;
  esac
}

runtime_release_arch() {
  case "$ARCH" in
    x86_64)  echo "linux-amd64" ;;
    aarch64) echo "linux-arm64" ;;
    *)       error "Unsupported architecture: $ARCH" ;;
  esac
}

builtin_runtime_filename() {
  case "$1" in
    node22) echo "node22.ext4" ;;
    node24) echo "node24.ext4" ;;
    python3.13) echo "python3.13.ext4" ;;
    *) error "Unsupported runtime: $1" ;;
  esac
}

is_truthy() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON)
      return 0
      ;;
    *)
      return 1
      ;;
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

resolve_script_dir() {
  local script_path="${BASH_SOURCE[0]:-$0}"
  cd "$(dirname "$script_path")" && pwd -P
}

resolve_source_dir() {
  local dir="$1"
  [ -n "$dir" ] || error "source directory is required"
  [ -d "$dir" ] || error "Source directory not found: $dir"
  (
    cd "$dir" && pwd -P
  )
}

validate_source_tree() {
  local dir="$1"
  [ -f "$dir/package.json" ] || error "Invalid source tree: missing $dir/package.json"
  [ -d "$dir/agent" ] || error "Invalid source tree: missing $dir/agent"
  [ -d "$dir/hostd" ] || error "Invalid source tree: missing $dir/hostd"
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

  exec 3<"$script_tmp"
  rm -f -- "$script_tmp"
  exec env "${env_args[@]}" bash -s -- "$@" <&3
}

while [ $# -gt 0 ]; do
  case "$1" in
    --uninstall)
      UNINSTALL=1
      shift
      ;;
    --local)
      REQUESTED_SOURCE_DIR="$(resolve_script_dir)"
      shift
      ;;
    --source-dir)
      [ $# -ge 2 ] || error "--source-dir requires a value"
      REQUESTED_SOURCE_DIR="$2"
      shift 2
      ;;
    --source-dir=*)
      REQUESTED_SOURCE_DIR="${1#--source-dir=}"
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
[ -n "$REQUESTED_SOURCE_DIR" ] && [ -n "$REQUESTED_REF" ] && error "Use only one of --local/--source-dir or --ref"
[ -n "$REQUESTED_SOURCE_DIR" ] && [ -n "$REQUESTED_SHA" ] && error "Use only one of --local/--source-dir or --sha"

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
SOURCE_LOCAL=0
VMSAN_SRC=""
if [ -n "$REQUESTED_SOURCE_DIR" ]; then
  INSTALL_MODE="source"
  SOURCE_LOCAL=1
  VMSAN_SRC="$(resolve_source_dir "$REQUESTED_SOURCE_DIR")"
  validate_source_tree "$VMSAN_SRC"
  SOURCE_SHA="$(git -C "$VMSAN_SRC" rev-parse HEAD 2>/dev/null || echo "local")"
  SOURCE_LABEL="$VMSAN_SRC"
elif [ -n "$VMSAN_INSTALL_SOURCE_SHA" ]; then
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

  # Stop and disable gateway service
  if command -v systemctl >/dev/null 2>&1; then
    systemctl stop vmsan-gateway 2>/dev/null || true
    systemctl disable vmsan-gateway 2>/dev/null || true
    rm -f /etc/systemd/system/vmsan-gateway.service
    systemctl daemon-reload 2>/dev/null || true
  fi

  # Remove system-wide binaries installed by the gateway setup
  rm -f /usr/local/bin/firecracker /usr/local/bin/jailer
  rm -f /usr/local/bin/vmsan-nftables /usr/local/bin/vmsan-gateway
  rm -rf /run/vmsan

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

ensure_docker() {
  if command -v docker >/dev/null 2>&1; then
    return
  fi

  info "Docker not found — installing Docker via get.docker.com..."
  curl -fsSL https://get.docker.com | sh
  command -v docker >/dev/null 2>&1 || error "Docker installation failed"
  success "Docker $(docker --version | awk '{print $3}' | tr -d ',') installed"
}

download_source_tree_ref() {
  local ref="$1"
  local dest="$2"

  rm -rf "$dest"
  mkdir -p "$dest"
  info "Fetching vmsan source at $ref..."
  curl -fsSL "https://github.com/$VMSAN_REPO/archive/${ref}.tar.gz" \
    | tar -xz --strip-components=1 -C "$dest"
}

MISSING_PKGS=()
command -v unzip       >/dev/null 2>&1 || MISSING_PKGS+=(unzip)
command -v unsquashfs  >/dev/null 2>&1 || MISSING_PKGS+=(squashfs-tools)
command -v mkfs.ext4   >/dev/null 2>&1 || MISSING_PKGS+=(e2fsprogs)
command -v zstd        >/dev/null 2>&1 || MISSING_PKGS+=(zstd)

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

# --- vmsan group + system directories ---

# Create vmsan system group early so /srv/jailer can have correct ownership.
# The vmsan group grants non-root users access to the gateway Unix socket.
groupadd -r vmsan 2>/dev/null || true
REAL_USER="${SUDO_USER:-$(whoami)}"
usermod -aG vmsan "$REAL_USER" 2>/dev/null || true

# Create gateway runtime directory with correct group ownership
mkdir -p /run/vmsan
chown root:vmsan /run/vmsan
chmod 775 /run/vmsan

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

  # Extract seccompiler-bin and official seccomp filter from the same tarball
  mkdir -p "$VMSAN_DIR/seccomp"
  if cp "$FC_TMP"/release-*/seccompiler-bin-*-"$ARCH" "$VMSAN_DIR/bin/seccompiler-bin" 2>/dev/null; then
    chmod +x "$VMSAN_DIR/bin/seccompiler-bin"
  fi
  if cp "$FC_TMP"/release-*/seccomp-filter-*-"$ARCH".json "$VMSAN_DIR/seccomp/firecracker-default.json" 2>/dev/null; then
    chmod 600 "$VMSAN_DIR/seccomp/firecracker-default.json"
  fi

  rm -rf "$FC_TMP"
  trap - EXIT
  success "Firecracker $FC_VER installed"
fi

# Install firecracker + jailer to system path (gateway runs as root, needs these)
install -m 0755 "$VMSAN_DIR/bin/firecracker" /usr/local/bin/firecracker
install -m 0755 "$VMSAN_DIR/bin/jailer" /usr/local/bin/jailer

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

  mkfs.ext4 -q -d "$SQUASHFS_TMP/rootfs" "$ROOTFS_PATH" "${IMAGE_MB}M"
  tune2fs -m 0 "$ROOTFS_PATH" >/dev/null 2>&1

  rm -rf "$SQUASHFS_TMP"
  trap - EXIT
  success "Rootfs installed ($ROOTFS_FILE, ${IMAGE_MB} MB)"
fi

# --- source / release selection ---

if [ -z "$VMSAN_SRC" ]; then
  VMSAN_SRC="$VMSAN_DIR/src"
fi

LATEST_TAG=""
LOCAL_RUNTIME_BUILD=0

if is_truthy "$FORCE_LOCAL_RUNTIME_BUILD"; then
  LOCAL_RUNTIME_BUILD=1
elif [ "$INSTALL_MODE" = "source" ] && [ "${SOURCE_LOCAL:-0}" -eq 0 ]; then
  LOCAL_RUNTIME_BUILD=1
fi

if [ "$INSTALL_MODE" = "source" ]; then
  if [ "$SOURCE_LOCAL" -eq 1 ]; then
    info "Local source install mode active (${SOURCE_LABEL})"
    if [ "$LOCAL_RUNTIME_BUILD" -eq 1 ]; then
      info "Local runtime build forced; built-in runtimes will be built from source"
    else
      info "Using public runtime artifacts from ${RUNTIME_MANIFEST_URL}"
    fi
  else
    VMSAN_SRC="$VMSAN_DIR/src"
    info "Source install mode active (${SOURCE_LABEL})"
    download_source_tree_ref "$SOURCE_SHA" "$VMSAN_SRC"
  fi
else
  info "Fetching latest release tag..."
  LATEST_TAG=$(curl -fsSL "https://api.github.com/repos/$VMSAN_REPO/releases" | grep -oP '"tag_name":\s*"\K[^"]+' | head -1)
  [ -n "$LATEST_TAG" ] || error "Could not determine latest release tag"
  success "Latest release: $LATEST_TAG"
  if [ "$LOCAL_RUNTIME_BUILD" -eq 1 ]; then
    info "Local runtime build forced; downloading source tree for ${LATEST_TAG}..."
    download_source_tree_ref "$LATEST_TAG" "$VMSAN_SRC"
  fi
fi

# --- vmsan CLI ---

if [ "$INSTALL_MODE" = "source" ]; then
  ensure_go
  info "Building vmsan from source (${SOURCE_SHA})..."
  (cd "$VMSAN_SRC/hostd" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go_arch)" go build -buildvcs=false \
    -ldflags="-s -w -X main.version=${SOURCE_SHA}" -o "$VMSAN_DIR/bin/vmsan" ./cmd/vmsan)
  chmod +x "$VMSAN_DIR/bin/vmsan"
  install -m 0755 "$VMSAN_DIR/bin/vmsan" /usr/local/bin/vmsan
  VMSAN_VER=$("$VMSAN_DIR/bin/vmsan" --version 2>/dev/null || echo "unknown")
  success "vmsan built from ${SOURCE_LABEL} ($VMSAN_VER)"
elif [ -x "$VMSAN_DIR/bin/vmsan" ]; then
  VMSAN_VER=$("$VMSAN_DIR/bin/vmsan" --version 2>/dev/null || echo "unknown")
  success "vmsan already installed ($VMSAN_VER)"
else
  VMSAN_URL="https://github.com/$VMSAN_REPO/releases/download/${LATEST_TAG}/vmsan-${ARCH}"
  download "$VMSAN_URL" "$VMSAN_DIR/bin/vmsan"
  chmod +x "$VMSAN_DIR/bin/vmsan"
  install -m 0755 "$VMSAN_DIR/bin/vmsan" /usr/local/bin/vmsan
  VMSAN_VER=$("$VMSAN_DIR/bin/vmsan" --version 2>/dev/null || echo "unknown")
  success "vmsan ${LATEST_TAG} installed ($VMSAN_VER)"
fi

# --- vmsan-agent ---

AGENT_PATH="$VMSAN_DIR/bin/vmsan-agent"

if [ "$INSTALL_MODE" = "source" ]; then
  ensure_go
  info "Building vmsan-agent from source (${SOURCE_SHA})..."
  (cd "$VMSAN_SRC/agent" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go_arch)" go build -buildvcs=false -ldflags="-s -w" -o "$AGENT_PATH" .)
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

# Backward-compat symlink: vmsan-gateway -> vmsan
ln -sf /usr/local/bin/vmsan /usr/local/bin/vmsan-gateway 2>/dev/null || true

# --- dnsproxy ---

DNSPROXY_VERSION="v0.73.3"
DNSPROXY_PATH="$VMSAN_DIR/bin/dnsproxy"

if [ -x "$DNSPROXY_PATH" ]; then
  success "dnsproxy already installed"
else
  DNSPROXY_ARCH="$(go_arch)"
  DNSPROXY_URL="https://github.com/AdguardTeam/dnsproxy/releases/download/${DNSPROXY_VERSION}/dnsproxy-linux-${DNSPROXY_ARCH}-${DNSPROXY_VERSION}.tar.gz"
  DNSPROXY_TMP="$(mktemp -d)"
  info "Downloading dnsproxy ${DNSPROXY_VERSION}..."
  if ! curl -fsSL -o "$DNSPROXY_TMP/dnsproxy.tar.gz" "$DNSPROXY_URL"; then
    rm -rf "$DNSPROXY_TMP"
    error "Failed to download dnsproxy from $DNSPROXY_URL"
  fi
  tar -xzf "$DNSPROXY_TMP/dnsproxy.tar.gz" -C "$DNSPROXY_TMP"
  # The archive contains linux-<arch>/dnsproxy
  find "$DNSPROXY_TMP" -name "dnsproxy" -type f -exec mv {} "$DNSPROXY_PATH" \;
  chmod +x "$DNSPROXY_PATH"
  rm -rf "$DNSPROXY_TMP"
  success "dnsproxy ${DNSPROXY_VERSION} installed"
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

# --- seccomp filter ---

SECCOMP_DIR="$VMSAN_DIR/seccomp"
SECCOMP_JSON="$SECCOMP_DIR/default.json"
SECCOMP_BPF="$SECCOMP_DIR/default.bpf"
SECCOMPILER_BIN="$VMSAN_DIR/bin/seccompiler-bin"

mkdir -p "$SECCOMP_DIR"

# Copy bundled filter (from source or Firecracker tarball)
if [ "$INSTALL_MODE" = "source" ] && [ -f "$VMSAN_SRC/seccomp/default.json" ]; then
  cp "$VMSAN_SRC/seccomp/default.json" "$SECCOMP_JSON"
  chmod 600 "$SECCOMP_JSON"
elif [ ! -f "$SECCOMP_JSON" ]; then
  # Use the official Firecracker filter extracted during FC download
  if [ -f "$SECCOMP_DIR/firecracker-default.json" ]; then
    cp "$SECCOMP_DIR/firecracker-default.json" "$SECCOMP_JSON"
    chmod 600 "$SECCOMP_JSON"
  fi
else
  chmod 600 "$SECCOMP_JSON" 2>/dev/null || true
fi

# Compile JSON to BPF using seccompiler-bin (extracted from Firecracker tarball)
SECCOMPILER=""
if [ -x "$SECCOMPILER_BIN" ]; then
  SECCOMPILER="$SECCOMPILER_BIN"
elif command -v seccompiler-bin >/dev/null 2>&1; then
  SECCOMPILER="seccompiler-bin"
fi

SECCOMP_ARCH="x86_64"
[ "$ARCH" = "aarch64" ] && SECCOMP_ARCH="aarch64"

# Compile the official Firecracker filter (preferred — most compatible)
FC_SECCOMP_JSON="$SECCOMP_DIR/firecracker-default.json"
FC_SECCOMP_BPF="$SECCOMP_DIR/firecracker-default.bpf"
if [ -n "$SECCOMPILER" ] && [ -f "$FC_SECCOMP_JSON" ] && [ ! -f "$FC_SECCOMP_BPF" ]; then
  if "$SECCOMPILER" --input-file "$FC_SECCOMP_JSON" --target-arch "$SECCOMP_ARCH" --output-file "$FC_SECCOMP_BPF" 2>/dev/null; then
    chmod 600 "$FC_SECCOMP_BPF"
    success "Seccomp BPF filter compiled (firecracker-default)"
  else
    warn "Seccomp BPF compilation failed — VMs will use Firecracker built-in filter"
  fi
fi

# Also compile the vmsan custom filter as fallback
if [ -n "$SECCOMPILER" ] && [ -f "$SECCOMP_JSON" ] && [ ! -f "$SECCOMP_BPF" ]; then
  if "$SECCOMPILER" --input-file "$SECCOMP_JSON" --target-arch "$SECCOMP_ARCH" --output-file "$SECCOMP_BPF" 2>/dev/null; then
    chmod 600 "$SECCOMP_BPF"
  fi
elif [ -z "$SECCOMPILER" ] && [ ! -f "$FC_SECCOMP_BPF" ] && [ ! -f "$SECCOMP_BPF" ]; then
  warn "seccompiler-bin not found — VMs will use Firecracker built-in filter"
fi

# Fix ownership for SUDO_USER
if [ -n "${SUDO_USER:-}" ]; then
  SUDO_HOME="$(getent passwd "$SUDO_USER" 2>/dev/null | cut -d: -f6 || true)"
  if [ -n "$SUDO_HOME" ] && [ -d "$SECCOMP_DIR" ]; then
    chown -R "$SUDO_USER":"$(id -gn "$SUDO_USER")" "$SECCOMP_DIR" 2>/dev/null || true
  fi
fi

# --- runtime images ---

BUILTIN_RUNTIMES=(node22 node24 python3.13)
HOST_RUNTIME_ARCH="$(runtime_release_arch)"
RUNTIME_RECIPE_VERSION_INSTALLED=""
RUNTIME_DISTRIBUTION="r2"

runtime_metadata_path() {
  local name="$1"
  echo "$VMSAN_DIR/rootfs/${name}.meta"
}

runtime_metadata_field() {
  local name="$1" key="$2"
  local meta
  meta="$(runtime_metadata_path "$name")"
  [ -f "$meta" ] || return 1
  sed -n "s/^${key}=//p" "$meta" | head -n1
}

runtime_metadata_matches() {
  local name="$1"
  shift
  local meta
  meta="$(runtime_metadata_path "$name")"
  [ -f "$meta" ] || return 1

  while [ $# -gt 0 ]; do
    local key="$1"
    local expected="$2"
    local actual
    shift 2
    actual="$(runtime_metadata_field "$name" "$key" || true)"
    [ "$actual" = "$expected" ] || return 1
  done
}

write_runtime_metadata() {
  local name="$1"
  shift
  local meta
  meta="$(runtime_metadata_path "$name")"

  : > "$meta"
  while [ $# -gt 0 ]; do
    printf '%s=%s\n' "$1" "$2" >> "$meta"
    shift 2
  done
}

read_runtime_manifest_entry() {
  local manifest_file="$1" runtime="$2" arch="$3"

  MANIFEST_PATH="$manifest_file" \
  MANIFEST_RUNTIME="$runtime" \
  MANIFEST_ARCH="$arch" \
  MANIFEST_SOURCE_URL="$RUNTIME_MANIFEST_URL" \
  node <<'EOF'
const fs = require("fs");

const fail = (message) => {
  console.error(message);
  process.exit(1);
};

const manifestPath = process.env.MANIFEST_PATH;
const runtime = process.env.MANIFEST_RUNTIME;
const arch = process.env.MANIFEST_ARCH;
const manifestSourceUrl = process.env.MANIFEST_SOURCE_URL || manifestPath;

let manifest;
try {
  manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
} catch (error) {
  fail(`Could not parse runtime manifest ${manifestSourceUrl}: ${error.message}`);
}

if (manifest.schemaVersion !== 1) {
  fail(`Unsupported runtime manifest schemaVersion ${manifest.schemaVersion} from ${manifestSourceUrl}`);
}

const entry = manifest.runtimes?.[runtime]?.[arch];
if (!entry) {
  fail(`Runtime "${runtime}" for "${arch}" is missing from ${manifestSourceUrl}`);
}

const values = {
  manifestVersion: String(manifest.version ?? ""),
  manifestRecipeVersion: String(manifest.recipeVersion ?? ""),
  artifactVersion: String(entry.artifactVersion ?? ""),
  entryRecipeVersion: String(entry.recipeVersion ?? ""),
  url: String(entry.url ?? ""),
  sha256: String(entry.sha256 ?? ""),
  bytes: String(entry.bytes ?? ""),
  baseImage: String(entry.baseImage ?? ""),
  baseImageDigest: String(entry.baseImageDigest ?? ""),
};

for (const [key, value] of Object.entries(values)) {
  if (!value) {
    fail(`Runtime "${runtime}" for "${arch}" is missing "${key}" in ${manifestSourceUrl}`);
  }
  console.log(`${key}=${value}`);
}
EOF
}

install_runtime_from_manifest() {
  local runtime="$1" arch="$2" manifest_file="$3"
  local entry_lines
  local manifest_version=""
  local manifest_recipe_version=""
  local artifact_version=""
  local entry_recipe_version=""
  local url=""
  local sha256=""
  local bytes=""
  local base_image=""
  local base_image_digest=""
  local dest
  local tmp_dir
  local artifact_tmp
  local ext4_tmp
  local actual_sha256
  local actual_bytes

  if ! entry_lines="$(read_runtime_manifest_entry "$manifest_file" "$runtime" "$arch")"; then
    error "Could not resolve runtime \"$runtime\" for \"$arch\" from $RUNTIME_MANIFEST_URL"
  fi

  while IFS='=' read -r key value; do
    case "$key" in
      manifestVersion) manifest_version="$value" ;;
      manifestRecipeVersion) manifest_recipe_version="$value" ;;
      artifactVersion) artifact_version="$value" ;;
      entryRecipeVersion) entry_recipe_version="$value" ;;
      url) url="$value" ;;
      sha256) sha256="$value" ;;
      bytes) bytes="$value" ;;
      baseImage) base_image="$value" ;;
      baseImageDigest) base_image_digest="$value" ;;
    esac
  done <<< "$entry_lines"

  [ "$manifest_recipe_version" = "$entry_recipe_version" ] \
    || error "Runtime manifest recipe version mismatch for $runtime ($manifest_recipe_version != $entry_recipe_version)"

  dest="$VMSAN_DIR/rootfs/$(builtin_runtime_filename "$runtime")"
  if [ -f "$dest" ] && runtime_metadata_matches \
    "$runtime" \
    distribution r2 \
    artifact_version "$artifact_version" \
    recipe_version "$entry_recipe_version" \
    sha256 "$sha256" \
    base_image "$base_image" \
    base_image_digest "$base_image_digest"; then
    RUNTIME_RECIPE_VERSION_INSTALLED="$entry_recipe_version"
    success "Runtime ${runtime} already installed (${artifact_version})"
    return
  fi

  info "Installing runtime ${runtime} from release ${manifest_version} (${arch})..."
  tmp_dir="$(mktemp -d)"
  artifact_tmp="$tmp_dir/rootfs.ext4.zst"
  ext4_tmp="$tmp_dir/$(builtin_runtime_filename "$runtime")"
  trap 'trap - RETURN; rm -rf "$tmp_dir"' RETURN

  download "$url" "$artifact_tmp"

  actual_sha256="$(sha256sum "$artifact_tmp" | awk '{print $1}')"
  [ "$actual_sha256" = "$sha256" ] \
    || error "Checksum mismatch for runtime ${runtime} from ${url} (expected ${sha256}, got ${actual_sha256})"

  actual_bytes="$(stat -c %s "$artifact_tmp")"
  [ "$actual_bytes" = "$bytes" ] \
    || error "Size mismatch for runtime ${runtime} from ${url} (expected ${bytes}, got ${actual_bytes})"

  zstd -d -q -f -o "$ext4_tmp" "$artifact_tmp"
  mv "$ext4_tmp" "$dest"

  write_runtime_metadata \
    "$runtime" \
    distribution r2 \
    artifact_version "$artifact_version" \
    recipe_version "$entry_recipe_version" \
    sha256 "$sha256" \
    base_image "$base_image" \
    base_image_digest "$base_image_digest" \
    installed_at "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

  RUNTIME_RECIPE_VERSION_INSTALLED="$entry_recipe_version"
  trap - RETURN
  rm -rf "$tmp_dir"
  success "Runtime ${runtime} installed from R2 (${artifact_version})"
}

build_runtime_from_source_tree() {
  local runtime="$1" source_tree="$2" artifact_version="$3" arch="$4"
  local runtime_manifest="$source_tree/docker/runtimes/runtime-manifest.sh"
  local build_dir
  local dest
  local base_image
  local base_image_digest
  local artifact_sha256
  local recipe_version
  local expected_recipe_version
  local metadata_json

  [ -f "$runtime_manifest" ] || error "Runtime manifest not found at $runtime_manifest"
  # shellcheck disable=SC1090
  source "$runtime_manifest"

  base_image="$(runtime_base_image "$runtime" 2>/dev/null)" \
    || error "Unsupported runtime in local build path: $runtime"
  expected_recipe_version="$RUNTIME_RECIPE_VERSION"
  dest="$VMSAN_DIR/rootfs/$(builtin_runtime_filename "$runtime")"

  if [ -f "$dest" ] && runtime_metadata_matches \
    "$runtime" \
    distribution local \
    artifact_version "$artifact_version" \
    recipe_version "$expected_recipe_version" \
    base_image "$base_image"; then
    RUNTIME_RECIPE_VERSION_INSTALLED="$expected_recipe_version"
    success "Runtime ${runtime} already built locally"
    return
  fi

  build_dir="$(mktemp -d)"
  trap 'trap - RETURN; rm -rf "$build_dir"' RETURN

  info "Building runtime ${runtime} locally..."
  bash "$source_tree/scripts/build-runtime-rootfs.sh" \
    --runtime "$runtime" \
    --arch "$arch" \
    --version "$artifact_version" \
    --output-dir "$build_dir"

  (
    cd "$build_dir"
    sha256sum -c sha256sums.txt >/dev/null
  ) || error "Checksum verification failed for locally built runtime ${runtime}"

  metadata_json="$build_dir/metadata.json"
  artifact_sha256="$(json_string_field sha256 "$metadata_json")"
  base_image_digest="$(json_string_field baseImageDigest "$metadata_json")"
  recipe_version="$(json_number_field recipeVersion "$metadata_json")"
  base_image="$(json_string_field baseImage "$metadata_json")"

  [ -n "$artifact_sha256" ] || error "Missing sha256 in $metadata_json"
  [ -n "$base_image_digest" ] || error "Missing baseImageDigest in $metadata_json"
  [ -n "$recipe_version" ] || error "Missing recipeVersion in $metadata_json"
  [ -n "$base_image" ] || error "Missing baseImage in $metadata_json"

  zstd -d -q -f -o "$build_dir/$(builtin_runtime_filename "$runtime")" "$build_dir/rootfs.ext4.zst"
  mv "$build_dir/$(builtin_runtime_filename "$runtime")" "$dest"

  write_runtime_metadata \
    "$runtime" \
    distribution local \
    artifact_version "$artifact_version" \
    recipe_version "$recipe_version" \
    sha256 "$artifact_sha256" \
    base_image "$base_image" \
    base_image_digest "$base_image_digest" \
    installed_at "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

  RUNTIME_RECIPE_VERSION_INSTALLED="$recipe_version"
  trap - RETURN
  rm -rf "$build_dir"
  success "Runtime ${runtime} built locally (${artifact_version})"
}

need_cmd zstd
need_cmd sha256sum

if [ "$LOCAL_RUNTIME_BUILD" -eq 1 ]; then
  RUNTIME_DISTRIBUTION="local"
  ensure_docker
  RUNTIME_ARTIFACT_VERSION="${LATEST_TAG:-$SOURCE_SHA}"
  for rt in "${BUILTIN_RUNTIMES[@]}"; do
    build_runtime_from_source_tree "$rt" "$VMSAN_SRC" "$RUNTIME_ARTIFACT_VERSION" "$HOST_RUNTIME_ARCH"
  done
else
  RUNTIME_MANIFEST_FILE="$(mktemp)"
  info "Fetching runtime manifest from $RUNTIME_MANIFEST_URL..."
  download "$RUNTIME_MANIFEST_URL" "$RUNTIME_MANIFEST_FILE"
  for rt in "${BUILTIN_RUNTIMES[@]}"; do
    install_runtime_from_manifest "$rt" "$HOST_RUNTIME_ARCH" "$RUNTIME_MANIFEST_FILE"
  done
  rm -f "$RUNTIME_MANIFEST_FILE"
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
cli_version=$("$VMSAN_DIR/bin/vmsan" --version 2>/dev/null || echo "unknown")
runtime_distribution=$RUNTIME_DISTRIBUTION
runtime_manifest_url=$RUNTIME_MANIFEST_URL
runtime_recipe_version=$RUNTIME_RECIPE_VERSION_INSTALLED
local_runtime_build=$LOCAL_RUNTIME_BUILD
source_dir=$VMSAN_SRC
installed_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF

# --- systemd service ---

if command -v systemctl >/dev/null 2>&1; then
  cat > /etc/systemd/system/vmsan-gateway.service <<SVCEOF
[Unit]
Description=vmsan gateway daemon
After=network.target

[Service]
Type=simple
ExecStartPre=/bin/mkdir -p /run/vmsan
ExecStartPre=/bin/chown root:vmsan /run/vmsan
ExecStartPre=/bin/chmod 775 /run/vmsan
ExecStart=/usr/local/bin/vmsan gateway
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
SVCEOF

  systemctl daemon-reload
  systemctl enable vmsan-gateway 2>/dev/null || true
  systemctl restart vmsan-gateway 2>/dev/null || systemctl start vmsan-gateway 2>/dev/null || true
  success "vmsan-gateway systemd service installed and started"

  REAL_USER="${SUDO_USER:-$(whoami)}"
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
RUNTIME_STATUS="${RUNTIME_STATUS% }"
RUNTIME_STATUS="${RUNTIME_STATUS:-none}"

echo ""
success "vmsan environment ready at $VMSAN_DIR"
echo ""
echo "  vmsan        $VMSAN_DIR/bin/vmsan"
echo "  Firecracker  $VMSAN_DIR/bin/firecracker"
echo "  Jailer       $VMSAN_DIR/bin/jailer"
echo "  Kernel       $VMSAN_DIR/kernels/$KERNEL_FILE"
echo "  Rootfs       $VMSAN_DIR/rootfs/$ROOTFS_FILE"
echo "  Runtimes     $RUNTIME_STATUS"
echo "  Agent        $AGENT_PATH"
echo "  cloudflared  $CLOUDFLARED_PATH ($CF_STATUS)"
echo "  CLI          /usr/local/bin/vmsan"
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

# --- next steps ---

REAL_USER="${SUDO_USER:-$(whoami)}"
if [ "$REAL_USER" != "root" ] && ! id -nG "$REAL_USER" 2>/dev/null | grep -qw vmsan; then
  echo ""
  echo "  \033[1;33m>>> IMPORTANT: Activate group membership before using vmsan <<<\033[0m"
  echo ""
  echo "  Your user ($REAL_USER) was added to the 'vmsan' group."
  echo "  This is required to communicate with the gateway daemon."
  echo ""
  echo "  Run one of these:"
  echo "    newgrp vmsan          # activate in current shell"
  echo "    # or log out and back in for permanent activation"
  echo ""
  echo "  Then verify with:  vmsan doctor"
  echo ""
elif [ "$REAL_USER" != "root" ]; then
  echo "  Ready to use: vmsan doctor"
  echo ""
fi
