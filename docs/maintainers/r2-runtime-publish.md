# Publish Built-In Runtimes to Cloudflare R2

Built-in runtime artifacts are published to Cloudflare R2 and served from `https://artifacts.vmsan.dev`.

This flow uses:

- `scripts/build-runtime-rootfs.sh` to build one runtime artifact
- `scripts/publish-runtimes-to-r2.sh` to build and upload release artifacts
- `rclone` for uploads

## Prerequisites

- Docker with Buildx
- QEMU and binfmt for cross-arch builds
- `rclone`
- an `rclone` remote for R2

Default assumptions:

- bucket: `vmsan-artifacts`
- public domain: `artifacts.vmsan.dev`
- `rclone` remote: `vmsan-r2:vmsan-artifacts`

If your remote name is different, set `VMSAN_R2_REMOTE`.

## Publish all runtimes

Upload a release without updating `stable.json`:

```bash
scripts/publish-runtimes-to-r2.sh \
  --version 0.1.1 \
  --channel stable \
  --runtime all \
  --arch all \
  --promote-stable false
```

Promote that release to `stable`:

```bash
scripts/publish-runtimes-to-r2.sh \
  --version 0.1.1 \
  --channel stable \
  --runtime all \
  --arch all \
  --promote-stable true
```

`stable.json` is uploaded last. If any build, checksum verification, or upload step fails, the script exits before updating the channel manifest.

## Publish a single runtime

```bash
scripts/publish-runtimes-to-r2.sh \
  --version 0.1.1 \
  --runtime node22 \
  --arch linux-amd64 \
  --promote-stable false
```

Promoting `stable` requires all runtimes for both `linux-amd64` and `linux-arm64`.

## Staging

Publish to a staging prefix:

```bash
VMSAN_R2_KEY_PREFIX=staging/runtimes \
VMSAN_ARTIFACTS_BASE_URL=https://artifacts.vmsan.dev/staging \
scripts/publish-runtimes-to-r2.sh \
  --version 0.1.1 \
  --runtime all \
  --arch all \
  --promote-stable false
```

Test the installer against that manifest:

```bash
curl -fsSL https://vmsan.dev/install | \
  VMSAN_RUNTIME_MANIFEST_URL=https://artifacts.vmsan.dev/staging/runtimes/releases/0.1.1/manifest.json bash
```

## Uploaded files

Each runtime and arch produces:

- `rootfs.ext4.zst`
- `metadata.json`
- `sha256sums.txt`

Release files are uploaded under `/runtimes/releases/<version>/...`.
The moving channel manifest is `/runtimes/channels/stable.json`.
