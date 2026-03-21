#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

ARCH="${1:-amd64}"
OUTPUT_DIR="${2:-$REPO_ROOT/dist/rootfs}"

info() { printf '[info] %s\n' "$*"; }
error() { printf '[error] %s\n' "$*" >&2; exit 1; }

case "$ARCH" in
  amd64) DOCKER_PLATFORM="linux/amd64" ;;
  arm64) DOCKER_PLATFORM="linux/arm64" ;;
  *) error "Unsupported architecture: $ARCH (use amd64 or arm64)" ;;
esac

command -v docker >/dev/null 2>&1 || error "docker is required"
command -v mkfs.ext4 >/dev/null 2>&1 || error "mkfs.ext4 is required"
docker buildx version >/dev/null 2>&1 || error "Docker Buildx is required"

BUILD_DIR="$(mktemp -d)"
CONTAINER_NAME="vmsan-rootfs-redis7-${ARCH}-$$"
BUILD_TAG="vmsan-rootfs-redis7:${ARCH}-$$"
OUTPUT_FILE="redis7-${ARCH}.ext4"

cleanup() {
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  docker image rm -f "$BUILD_TAG" >/dev/null 2>&1 || true
  rm -rf "$BUILD_DIR"
}
trap cleanup EXIT

info "Building Redis 7 rootfs for $ARCH"

cat > "$BUILD_DIR/Dockerfile" <<'DOCKERFILE'
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# Install Redis 7 from official repo
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl gnupg lsb-release \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://packages.redis.io/gpg \
      | gpg --dearmor -o /etc/apt/keyrings/redis-archive-keyring.gpg \
    && chmod 644 /etc/apt/keyrings/redis-archive-keyring.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/redis-archive-keyring.gpg] \
      https://packages.redis.io/deb $(lsb_release -cs) main" \
      > /etc/apt/sources.list.d/redis.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
      redis-server \
      systemd systemd-sysv \
      sudo procps \
    && rm -rf /var/lib/apt/lists/*

# Configure Redis for mesh access
RUN sed -i 's/^bind .*/bind 0.0.0.0/' /etc/redis/redis.conf && \
    sed -i 's/^protected-mode yes/protected-mode no/' /etc/redis/redis.conf

# Enable Redis auto-start via systemd
RUN systemctl enable redis-server

# Clean up unnecessary systemd units
RUN rm -f /etc/systemd/system/*.wants/* \
    /lib/systemd/system/multi-user.target.wants/* \
    /lib/systemd/system/local-fs.target.wants/* \
    /lib/systemd/system/sockets.target.wants/*udev* \
    /lib/systemd/system/sockets.target.wants/*initctl* 2>/dev/null || true && \
    systemctl enable redis-server
DOCKERFILE

docker buildx build \
  --platform "$DOCKER_PLATFORM" \
  --load \
  -t "$BUILD_TAG" \
  -f "$BUILD_DIR/Dockerfile" \
  "$BUILD_DIR" || error "Docker build failed"

docker create --name "$CONTAINER_NAME" "$BUILD_TAG" >/dev/null
docker export "$CONTAINER_NAME" -o "$BUILD_DIR/rootfs.tar" >/dev/null

info "Extracting filesystem"
mkdir -p "$BUILD_DIR/rootfs-extracted"
tar -xf "$BUILD_DIR/rootfs.tar" -C "$BUILD_DIR/rootfs-extracted"

TAR_BYTES="$(stat -c %s "$BUILD_DIR/rootfs.tar")"
IMAGE_MB=$(( TAR_BYTES / 1024 / 1024 + 512 ))
[ "$IMAGE_MB" -lt 1024 ] && IMAGE_MB=1024

info "Creating ext4 image (${IMAGE_MB}MB)"
mkfs.ext4 -q -d "$BUILD_DIR/rootfs-extracted" "$BUILD_DIR/rootfs.ext4" "${IMAGE_MB}M"
tune2fs -m 0 "$BUILD_DIR/rootfs.ext4" >/dev/null 2>&1

mkdir -p "$OUTPUT_DIR"
cp "$BUILD_DIR/rootfs.ext4" "$OUTPUT_DIR/$OUTPUT_FILE"

SHA256="$(sha256sum "$OUTPUT_DIR/$OUTPUT_FILE" | awk '{print $1}')"
info "Output: $OUTPUT_DIR/$OUTPUT_FILE"
info "SHA256: $SHA256"
info "Done"
