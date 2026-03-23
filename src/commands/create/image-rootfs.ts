import { consola } from "consola";
import { execSync } from "node:child_process";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { ImageReference } from "./validation.ts";
import { dockerUnavailableError } from "./docker-errors.ts";
import { toError } from "../../lib/utils.ts";
import { GatewayClient } from "../../lib/gateway-client.ts";

const APT_PACKAGES = [
  "bind9-utils",
  "bzip2",
  "findutils",
  "git",
  "gzip",
  "iptables",
  "iputils-ping",
  "libicu-dev",
  "libjpeg-dev",
  "libpng-dev",
  "ncurses-base",
  "libssl-dev",
  "openssl",
  "procps",
  "sudo",
  "systemd",
  "systemd-sysv",
  "tar",
  "unzip",
  "debianutils",
  "whois",
  "zstd",
];

const DNF_PACKAGES = [
  "bind-utils",
  "bzip2",
  "findutils",
  "git",
  "gzip",
  "iptables",
  "iputils",
  "libicu",
  "libjpeg",
  "libpng",
  "ncurses-libs",
  "openssl",
  "openssl-libs",
  "procps",
  "sudo",
  "tar",
  "unzip",
  "which",
  "whois",
  "zstd",
];

const APK_PACKAGES = [
  "bash",
  "bind-tools",
  "bzip2",
  "findutils",
  "git",
  "gzip",
  "iptables",
  "iputils",
  "icu-libs",
  "libjpeg-turbo",
  "libpng",
  "ncurses-libs",
  "openrc",
  "openssl",
  "procps",
  "sudo",
  "tar",
  "unzip",
  "whois",
  "zstd",
];

function generateDockerfile(baseImage: string, minimal = false): string {
  if (minimal) return `FROM ${baseImage}\n`;

  const aptInstall = `apt-get update && apt-get install -y --no-install-recommends ${APT_PACKAGES.join(" ")} && rm -rf /var/lib/apt/lists/*`;
  const dnfInstall = `dnf install -y ${DNF_PACKAGES.join(" ")} && dnf clean all`;
  const yumInstall = `yum install -y ${DNF_PACKAGES.join(" ")} && yum clean all`;
  const apkInstall = `apk add --no-cache ${APK_PACKAGES.join(" ")}`;

  return `FROM ${baseImage}
RUN if command -v apt-get >/dev/null 2>&1; then ${aptInstall}; \\
    elif command -v dnf >/dev/null 2>&1; then ${dnfInstall}; \\
    elif command -v yum >/dev/null 2>&1; then ${yumInstall}; \\
    elif command -v apk >/dev/null 2>&1; then ${apkInstall}; \\
    fi
RUN if command -v apk >/dev/null 2>&1; then \\
      id -u ubuntu >/dev/null 2>&1 || adduser -D -s /bin/bash ubuntu; \\
    else \\
      id -u ubuntu >/dev/null 2>&1 || useradd -m -s /bin/bash ubuntu; \\
    fi; \\
    echo 'ubuntu ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/ubuntu; \\
    chmod 440 /etc/sudoers.d/ubuntu; \\
    chown -R ubuntu:ubuntu /home/ubuntu
RUN EXTRA=""; \\
    if command -v npm >/dev/null 2>&1; then \\
      su -c 'mkdir -p /home/ubuntu/.npm-global && npm config set prefix /home/ubuntu/.npm-global' ubuntu; \\
      EXTRA="\${EXTRA}export PATH=\\"/home/ubuntu/.npm-global/bin:\\$PATH\\"\\n"; \\
    fi; \\
    if command -v pip3 >/dev/null 2>&1 || command -v pip >/dev/null 2>&1; then \\
      EXTRA="\${EXTRA}export PATH=\\"/home/ubuntu/.local/bin:\\$PATH\\"\\n"; \\
    fi; \\
    if [ -n "$EXTRA" ]; then \\
      printf '%b' "$EXTRA" >> /home/ubuntu/.profile; \\
      { printf '%b' "$EXTRA"; cat /home/ubuntu/.bashrc; } > /home/ubuntu/.bashrc.tmp \\
        && mv /home/ubuntu/.bashrc.tmp /home/ubuntu/.bashrc; \\
    fi; \\
    chown -R ubuntu:ubuntu /home/ubuntu
RUN if command -v rc-update >/dev/null 2>&1; then \\
      rc-update add devfs sysinit 2>/dev/null || true; \\
      rc-update add mdev sysinit 2>/dev/null || true; \\
      rc-update add hwdrivers sysinit 2>/dev/null || true; \\
      rc-update add modules boot 2>/dev/null || true; \\
      rc-update add sysctl boot 2>/dev/null || true; \\
      rc-update add hostname boot 2>/dev/null || true; \\
      rc-update add bootmisc boot 2>/dev/null || true; \\
      rc-update add networking boot 2>/dev/null || true; \\
      printf '%s\\n' '::sysinit:/sbin/openrc sysinit' '::sysinit:/sbin/openrc boot' '::wait:/sbin/openrc default' '::shutdown:/sbin/openrc shutdown' 'ttyS0::respawn:/sbin/getty 115200 ttyS0' > /etc/inittab; \\
    fi
`;
}

function verifyDocker(): void {
  try {
    execSync("docker info", { stdio: "pipe" });
  } catch {
    throw dockerUnavailableError();
  }
}

export function resolveImageRootfs(
  imageRef: ImageReference,
  registryDir: string,
  minimal = false,
): string {
  const cacheSuffix = minimal ? "-minimal" : "";
  const cacheDir = join(registryDir, `${imageRef.cacheKey}${cacheSuffix}`);
  const ext4Path = join(cacheDir, "rootfs.ext4");

  if (existsSync(ext4Path)) {
    consola.info(`Using cached rootfs for ${imageRef.full}`);
    return ext4Path;
  }

  // Cache miss — build the Docker image (not privileged, just needs docker group)
  verifyDocker();
  mkdirSync(cacheDir, { recursive: true });

  const buildTag = `vmsan-rootfs-${imageRef.name.replace(/[^a-z0-9._-]/gi, "-")}:${imageRef.tag}`;

  consola.start(`Building image from ${imageRef.full}...`);
  const dockerfile = generateDockerfile(imageRef.full, minimal);
  try {
    execSync(`docker build -t "${buildTag}" -f - . <<'DOCKERFILE'\n${dockerfile}\nDOCKERFILE`, {
      stdio: "pipe",
      shell: "/bin/bash",
    });
  } catch (err) {
    throw new Error(`Docker build failed: ${toError(err).message}`);
  }

  // Delegate the privileged ext4 conversion to the gateway daemon
  // (docker create/export + mkfs.ext4 + mount + copy + chown)
  consola.start("Converting to ext4 via gateway...");
  const gateway = new GatewayClient();

  // Use sync wrapper since resolveImageRootfs is called synchronously
  // Fall back to direct build if gateway is unavailable
  try {
    const { execSync: execSyncImport } = require("node:child_process");
    const request = JSON.stringify({
      method: "rootfs.build",
      params: { imageRef: buildTag, outputDir: cacheDir },
    });
    execSyncImport(
      `echo '${request.replace(/'/g, "'\\''")}' | socat - UNIX-CONNECT:/run/vmsan/gateway.sock`,
      { stdio: "pipe", timeout: 120000 },
    );
  } catch {
    // Gateway unavailable — fall back to direct build (needs sudo)
    consola.debug("Gateway unavailable for rootfs build, falling back to direct build");
    const containerName = `vmsan-export-${Date.now()}`;
    const tmpTar = join(cacheDir, "rootfs.tar");

    try {
      consola.start("Exporting filesystem...");
      execSync(`docker create --name "${containerName}" "${buildTag}"`, { stdio: "pipe" });
      execSync(`docker export "${containerName}" -o "${tmpTar}"`, { stdio: "pipe" });

      consola.start("Converting to ext4...");
      const tarSizeOutput = execSync(`stat -c %s "${tmpTar}"`, { encoding: "utf-8" }).trim();
      const tarBytes = Number(tarSizeOutput);
      const tarMb = tarBytes / 1024 / 1024;
      const imageSizeMb = Math.max(1024, Math.ceil(tarMb + 512));

      const tmpExtract = join(cacheDir, "rootfs-extracted");
      mkdirSync(tmpExtract, { recursive: true });
      execSync(`tar -xf "${tmpTar}" -C "${tmpExtract}"`, { stdio: "pipe" });
      execSync(`mkfs.ext4 -q -d "${tmpExtract}" "${ext4Path}" "${imageSizeMb}M"`, {
        stdio: "pipe",
      });
      execSync(`tune2fs -m 0 "${ext4Path}"`, { stdio: "pipe" });
      execSync(`rm -rf "${tmpExtract}" "${tmpTar}"`, { stdio: "pipe" });
    } finally {
      try {
        execSync(`docker rm -f "${containerName}" 2>/dev/null`, { stdio: "pipe" });
      } catch (err) {
        consola.debug(`Failed to remove docker container: ${toError(err).message}`);
      }
    }
  }

  writeFileSync(
    join(cacheDir, "metadata.json"),
    JSON.stringify({ image: imageRef.full, builtAt: new Date().toISOString() }, null, 2),
  );

  consola.success(`Rootfs built from ${imageRef.full}`);
  return ext4Path;
}
