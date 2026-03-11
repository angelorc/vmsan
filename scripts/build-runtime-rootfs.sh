#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

source "$REPO_ROOT/docker/runtimes/runtime-manifest.sh"

usage() {
  cat <<'EOF'
Usage:
  scripts/build-runtime-rootfs.sh --runtime <name> --arch <linux-amd64|linux-arm64> --version <version> --output-dir <dir>
EOF
}

info() {
  printf '[info] %s\n' "$*"
}

error() {
  printf '[error] %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || error "Required command not found: $1"
}

json_escape() {
  local value="${1:-}"
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  value=${value//$'\t'/\\t}
  printf '%s' "$value"
}

RUNTIME=""
ARCH=""
VERSION=""
OUTPUT_DIR=""

while [ $# -gt 0 ]; do
  case "$1" in
    --runtime)
      [ $# -ge 2 ] || error "--runtime requires a value"
      RUNTIME="$2"
      shift 2
      ;;
    --arch)
      [ $# -ge 2 ] || error "--arch requires a value"
      ARCH="$2"
      shift 2
      ;;
    --version)
      [ $# -ge 2 ] || error "--version requires a value"
      VERSION="$2"
      shift 2
      ;;
    --output-dir)
      [ $# -ge 2 ] || error "--output-dir requires a value"
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      error "Unknown argument: $1"
      ;;
  esac
done

[ -n "$RUNTIME" ] || error "--runtime is required"
[ -n "$ARCH" ] || error "--arch is required"
[ -n "$VERSION" ] || error "--version is required"
[ -n "$OUTPUT_DIR" ] || error "--output-dir is required"

BASE_IMAGE="$(runtime_base_image "$RUNTIME" 2>/dev/null)" \
  || error "Unsupported runtime: $RUNTIME"
RUNTIME_FILENAME="$(runtime_filename "$RUNTIME" 2>/dev/null)" \
  || error "Unsupported runtime filename mapping: $RUNTIME"
ARCH_DIR="$(runtime_arch_dir "$ARCH" 2>/dev/null)" \
  || error "Unsupported architecture: $ARCH"
DOCKER_PLATFORM="$(runtime_docker_platform "$ARCH_DIR" 2>/dev/null)" \
  || error "Could not resolve Docker platform for $ARCH_DIR"

need_cmd docker
need_cmd mkfs.ext4
need_cmd tune2fs
need_cmd tar
need_cmd zstd
need_cmd sha256sum
need_cmd sed
need_cmd stat

docker buildx version >/dev/null 2>&1 || error "Docker Buildx is required"

mkdir -p "$OUTPUT_DIR"
OUTPUT_DIR="$(cd "$OUTPUT_DIR" && pwd)"

ROOTFS_ZST="$OUTPUT_DIR/rootfs.ext4.zst"
METADATA_JSON="$OUTPUT_DIR/metadata.json"
SHA256SUMS="$OUTPUT_DIR/sha256sums.txt"

rm -f "$ROOTFS_ZST" "$METADATA_JSON" "$SHA256SUMS"

BUILD_DIR="$(mktemp -d)"
CONTAINER_NAME="vmsan-export-${RUNTIME//[^a-zA-Z0-9_.-]/-}-${ARCH_DIR##*-}-$$"
BUILD_TAG="vmsan-runtime-${RUNTIME//[^a-zA-Z0-9_.-]/-}:$(printf '%s' "${ARCH_DIR##*-}-$VERSION-$$" | tr '/:@' '---')"

cleanup() {
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  docker image rm -f "$BUILD_TAG" >/dev/null 2>&1 || true
  rm -rf "$BUILD_DIR"
}
trap cleanup EXIT

sed "s|__BASE_IMAGE__|${BASE_IMAGE}|g" \
  "$REPO_ROOT/docker/runtimes/Dockerfile.template" > "$BUILD_DIR/Dockerfile"

BASE_IMAGE_DIGEST="$(
  docker buildx imagetools inspect "$BASE_IMAGE" 2>/dev/null \
    | sed -n 's/^Digest:[[:space:]]*//p' \
    | head -n1
)"
[ -n "$BASE_IMAGE_DIGEST" ] || error "Could not resolve base image digest for $BASE_IMAGE"

info "Building $RUNTIME for $ARCH_DIR from $BASE_IMAGE"
docker buildx build \
  --platform "$DOCKER_PLATFORM" \
  --load \
  -t "$BUILD_TAG" \
  -f "$BUILD_DIR/Dockerfile" \
  "$BUILD_DIR" >/dev/null

docker create --name "$CONTAINER_NAME" "$BUILD_TAG" >/dev/null
docker export "$CONTAINER_NAME" -o "$BUILD_DIR/rootfs.tar" >/dev/null

mkdir -p "$BUILD_DIR/rootfs-extracted"
tar -xf "$BUILD_DIR/rootfs.tar" -C "$BUILD_DIR/rootfs-extracted"

TAR_BYTES="$(stat -c %s "$BUILD_DIR/rootfs.tar")"
IMAGE_MB=$(( TAR_BYTES / 1024 / 1024 + 512 ))
[ "$IMAGE_MB" -lt 1024 ] && IMAGE_MB=1024

mkfs.ext4 -q -d "$BUILD_DIR/rootfs-extracted" "$BUILD_DIR/rootfs.ext4" "${IMAGE_MB}M"
tune2fs -m 0 "$BUILD_DIR/rootfs.ext4" >/dev/null 2>&1

zstd -q -T0 -19 -f "$BUILD_DIR/rootfs.ext4" -o "$ROOTFS_ZST"

ARTIFACT_BYTES="$(stat -c %s "$ROOTFS_ZST")"
ARTIFACT_SHA256="$(sha256sum "$ROOTFS_ZST" | awk '{print $1}')"
GENERATED_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

cat > "$METADATA_JSON" <<EOF
{
  "schemaVersion": 1,
  "runtime": "$(json_escape "$RUNTIME")",
  "arch": "$(json_escape "$ARCH_DIR")",
  "dockerPlatform": "$(json_escape "$DOCKER_PLATFORM")",
  "filename": "$(json_escape "$RUNTIME_FILENAME")",
  "artifactVersion": "$(json_escape "$VERSION")",
  "recipeVersion": $RUNTIME_RECIPE_VERSION,
  "baseImage": "$(json_escape "$BASE_IMAGE")",
  "baseImageDigest": "$(json_escape "$BASE_IMAGE_DIGEST")",
  "bytes": $ARTIFACT_BYTES,
  "sha256": "$(json_escape "$ARTIFACT_SHA256")",
  "builtAt": "$(json_escape "$GENERATED_AT")"
}
EOF

(cd "$OUTPUT_DIR" && sha256sum rootfs.ext4.zst metadata.json > sha256sums.txt)

info "Runtime artifact ready: $ROOTFS_ZST"
