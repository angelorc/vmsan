#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

source "$REPO_ROOT/docker/runtimes/runtime-manifest.sh"

usage() {
  cat <<'EOF'
Usage:
  scripts/publish-runtimes-to-r2.sh --version <version> [--channel stable] [--runtime all|node22|node24|python3.13] [--arch all|linux-amd64|linux-arm64] [--promote-stable true|false]

Environment:
  VMSAN_R2_REMOTE             Rclone remote path. Default: vmsan-r2:vmsan-artifacts
  VMSAN_R2_KEY_PREFIX         Object prefix inside the bucket. Default: runtimes
  VMSAN_ARTIFACTS_BASE_URL    Public base URL. Default: https://artifacts.vmsan.dev
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

json_string_field() {
  local key="$1" file="$2"
  grep -oP "\"$key\"\\s*:\\s*\"\\K[^\"]+" "$file" 2>/dev/null | head -n1 || true
}

json_number_field() {
  local key="$1" file="$2"
  grep -oP "\"$key\"\\s*:\\s*\\K[0-9]+" "$file" 2>/dev/null | head -n1 || true
}

build_public_url() {
  local path="$1"
  printf '%s/%s\n' "${VMSAN_ARTIFACTS_BASE_URL%/}" "${path#/}"
}

build_remote_path() {
  local path="$1"
  printf '%s/%s\n' "${VMSAN_R2_REMOTE%/}" "${path#/}"
}

upload_object() {
  local src="$1"
  local remote_key="$2"
  local cache_control="$3"
  local content_type="$4"

  rclone copyto \
    "$src" \
    "$(build_remote_path "$remote_key")" \
    --metadata-set "cache-control=$cache_control" \
    --metadata-set "content-type=$content_type" >/dev/null
}

VERSION=""
CHANNEL="stable"
RUNTIME_SELECTOR="all"
ARCH_SELECTOR="all"
PROMOTE_STABLE="false"

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      [ $# -ge 2 ] || error "--version requires a value"
      VERSION="$2"
      shift 2
      ;;
    --channel)
      [ $# -ge 2 ] || error "--channel requires a value"
      CHANNEL="$2"
      shift 2
      ;;
    --runtime)
      [ $# -ge 2 ] || error "--runtime requires a value"
      RUNTIME_SELECTOR="$2"
      shift 2
      ;;
    --arch)
      [ $# -ge 2 ] || error "--arch requires a value"
      ARCH_SELECTOR="$2"
      shift 2
      ;;
    --promote-stable)
      [ $# -ge 2 ] || error "--promote-stable requires a value"
      PROMOTE_STABLE="$2"
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

[ -n "$VERSION" ] || error "--version is required"
[ "$CHANNEL" = "stable" ] || error "Only --channel stable is supported in this milestone"
[ "$PROMOTE_STABLE" = "true" ] || [ "$PROMOTE_STABLE" = "false" ] || error "--promote-stable must be true or false"

need_cmd docker
need_cmd rclone
need_cmd sha256sum

VMSAN_R2_REMOTE="${VMSAN_R2_REMOTE:-vmsan-r2:vmsan-artifacts}"
VMSAN_R2_KEY_PREFIX="${VMSAN_R2_KEY_PREFIX:-runtimes}"
VMSAN_ARTIFACTS_BASE_URL="${VMSAN_ARTIFACTS_BASE_URL:-https://artifacts.vmsan.dev}"

SELECTED_RUNTIMES=()
if [ "$RUNTIME_SELECTOR" = "all" ]; then
  mapfile -t SELECTED_RUNTIMES < <(runtime_list)
else
  runtime_base_image "$RUNTIME_SELECTOR" >/dev/null 2>&1 || error "Unsupported runtime: $RUNTIME_SELECTOR"
  SELECTED_RUNTIMES=("$RUNTIME_SELECTOR")
fi

SELECTED_ARCHES=()
if [ "$ARCH_SELECTOR" = "all" ]; then
  mapfile -t SELECTED_ARCHES < <(runtime_arch_list)
else
  SELECTED_ARCH="$(
    runtime_arch_dir "$ARCH_SELECTOR" 2>/dev/null
  )" || error "Unsupported architecture: $ARCH_SELECTOR"
  SELECTED_ARCHES=("$SELECTED_ARCH")
fi

if [ "$PROMOTE_STABLE" = "true" ]; then
  mapfile -t ALL_RUNTIMES < <(runtime_list)
  mapfile -t ALL_ARCHES < <(runtime_arch_list)
  [ "${#SELECTED_RUNTIMES[@]}" -eq "${#ALL_RUNTIMES[@]}" ] || error "Promoting stable requires all runtimes"
  [ "${#SELECTED_ARCHES[@]}" -eq "${#ALL_ARCHES[@]}" ] || error "Promoting stable requires linux-amd64 and linux-arm64"
fi

WORK_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

IMMUTABLE_CACHE_CONTROL="public, max-age=31536000, immutable"
CHANNEL_CACHE_CONTROL="public, max-age=60, must-revalidate"
GENERATED_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

for runtime in "${SELECTED_RUNTIMES[@]}"; do
  for arch in "${SELECTED_ARCHES[@]}"; do
    out_dir="$WORK_DIR/$runtime/$arch"
    mkdir -p "$out_dir"
    bash "$REPO_ROOT/scripts/build-runtime-rootfs.sh" \
      --runtime "$runtime" \
      --arch "$arch" \
      --version "$VERSION" \
      --output-dir "$out_dir"

    (
      cd "$out_dir"
      sha256sum -c sha256sums.txt >/dev/null
    ) || error "Checksum verification failed for $runtime $arch"
  done
done

RELEASE_MANIFEST="$WORK_DIR/manifest.json"

{
  printf '{\n'
  printf '  "schemaVersion": 1,\n'
  printf '  "channel": "stable",\n'
  printf '  "version": "%s",\n' "$VERSION"
  printf '  "generatedAt": "%s",\n' "$GENERATED_AT"
  printf '  "recipeVersion": %s,\n' "$RUNTIME_RECIPE_VERSION"
  printf '  "runtimes": {\n'

  first_runtime=1
  for runtime in "${SELECTED_RUNTIMES[@]}"; do
    [ $first_runtime -eq 1 ] || printf ',\n'
    first_runtime=0
    printf '    "%s": {\n' "$runtime"

    first_arch=1
    for arch in "${SELECTED_ARCHES[@]}"; do
      metadata="$WORK_DIR/$runtime/$arch/metadata.json"
      sha256="$(json_string_field sha256 "$metadata")"
      bytes="$(json_number_field bytes "$metadata")"
      base_image="$(json_string_field baseImage "$metadata")"
      base_image_digest="$(json_string_field baseImageDigest "$metadata")"
      artifact_version="$(json_string_field artifactVersion "$metadata")"
      recipe_version="$(json_number_field recipeVersion "$metadata")"
      release_key="$VMSAN_R2_KEY_PREFIX/releases/$VERSION/$runtime/$arch/rootfs.ext4.zst"

      [ -n "$sha256" ] || error "Missing sha256 in $metadata"
      [ -n "$bytes" ] || error "Missing bytes in $metadata"
      [ -n "$base_image" ] || error "Missing baseImage in $metadata"
      [ -n "$base_image_digest" ] || error "Missing baseImageDigest in $metadata"
      [ -n "$artifact_version" ] || error "Missing artifactVersion in $metadata"
      [ -n "$recipe_version" ] || error "Missing recipeVersion in $metadata"

      [ $first_arch -eq 1 ] || printf ',\n'
      first_arch=0
      printf '      "%s": {\n' "$arch"
      printf '        "url": "%s",\n' "$(build_public_url "$release_key")"
      printf '        "sha256": "%s",\n' "$sha256"
      printf '        "bytes": %s,\n' "$bytes"
      printf '        "baseImage": "%s",\n' "$base_image"
      printf '        "baseImageDigest": "%s",\n' "$base_image_digest"
      printf '        "artifactVersion": "%s",\n' "$artifact_version"
      printf '        "recipeVersion": %s\n' "$recipe_version"
      printf '      }'
    done

    printf '\n    }'
  done

  printf '\n  }\n'
  printf '}\n'
} > "$RELEASE_MANIFEST"

info "Uploading versioned runtime artifacts to R2"
for runtime in "${SELECTED_RUNTIMES[@]}"; do
  for arch in "${SELECTED_ARCHES[@]}"; do
    prefix="$VMSAN_R2_KEY_PREFIX/releases/$VERSION/$runtime/$arch"
    upload_object "$WORK_DIR/$runtime/$arch/rootfs.ext4.zst" "$prefix/rootfs.ext4.zst" "$IMMUTABLE_CACHE_CONTROL" "application/zstd"
    upload_object "$WORK_DIR/$runtime/$arch/metadata.json" "$prefix/metadata.json" "$IMMUTABLE_CACHE_CONTROL" "application/json"
    upload_object "$WORK_DIR/$runtime/$arch/sha256sums.txt" "$prefix/sha256sums.txt" "$IMMUTABLE_CACHE_CONTROL" "text/plain"
  done
done

upload_object "$RELEASE_MANIFEST" "$VMSAN_R2_KEY_PREFIX/releases/$VERSION/manifest.json" "$IMMUTABLE_CACHE_CONTROL" "application/json"

if [ "$PROMOTE_STABLE" = "true" ]; then
  info "Promoting $VERSION to stable"
  upload_object "$RELEASE_MANIFEST" "$VMSAN_R2_KEY_PREFIX/channels/stable.json" "$CHANNEL_CACHE_CONTROL" "application/json"
fi

info "Runtime publish completed for version $VERSION"
