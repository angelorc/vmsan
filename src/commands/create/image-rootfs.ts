import { consola } from "consola";
import { execSync } from "node:child_process";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { ImageReference } from "./validation.ts";
import { dockerUnavailableError } from "./docker-errors.ts";

const APT_PACKAGES = [
  "bind9-utils",
  "bzip2",
  "findutils",
  "git",
  "gzip",
  "iputils-ping",
  "libicu-dev",
  "libjpeg-dev",
  "libpng-dev",
  "ncurses-base",
  "libssl-dev",
  "openssh-server",
  "openssl",
  "procps",
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
  "iputils",
  "libicu",
  "libjpeg",
  "libpng",
  "ncurses-libs",
  "openssh-server",
  "openssl",
  "openssl-libs",
  "procps",
  "tar",
  "unzip",
  "which",
  "whois",
  "zstd",
];

const APK_PACKAGES = [
  "bind-tools",
  "bzip2",
  "findutils",
  "git",
  "gzip",
  "iputils",
  "icu-libs",
  "libjpeg-turbo",
  "libpng",
  "ncurses-libs",
  "openrc",
  "openssh",
  "openssl",
  "procps",
  "tar",
  "unzip",
  "whois",
  "zstd",
];

function generateDockerfile(baseImage: string): string {
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
RUN ssh-keygen -A 2>/dev/null || true; \\
    mkdir -p /root/.ssh && chmod 700 /root/.ssh; \\
    if [ -f /etc/ssh/sshd_config ]; then \\
      sed -i 's/^#*PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config; \\
    fi; \\
    if command -v rc-update >/dev/null 2>&1; then \\
      rc-update add devfs sysinit 2>/dev/null || true; \\
      rc-update add mdev sysinit 2>/dev/null || true; \\
      rc-update add hwdrivers sysinit 2>/dev/null || true; \\
      rc-update add modules boot 2>/dev/null || true; \\
      rc-update add sysctl boot 2>/dev/null || true; \\
      rc-update add hostname boot 2>/dev/null || true; \\
      rc-update add bootmisc boot 2>/dev/null || true; \\
      rc-update add networking boot 2>/dev/null || true; \\
      rc-update add sshd default 2>/dev/null || true; \\
      printf '%s\\n' '::sysinit:/sbin/openrc sysinit' '::sysinit:/sbin/openrc boot' '::wait:/sbin/openrc default' '::shutdown:/sbin/openrc shutdown' 'ttyS0::respawn:/sbin/getty 115200 ttyS0' > /etc/inittab; \\
    fi; \\
    if command -v systemctl >/dev/null 2>&1; then systemctl enable sshd 2>/dev/null || systemctl enable ssh 2>/dev/null || true; fi
`;
}

function verifyDocker(): void {
  try {
    execSync("docker info", { stdio: "pipe" });
  } catch {
    throw dockerUnavailableError();
  }
}

function buildImageRootfs(imageRef: ImageReference, cacheDir: string): string {
  const ext4Path = join(cacheDir, "rootfs.ext4");

  verifyDocker();

  const buildTag = `vmsan-rootfs-${imageRef.name.replace(/[^a-z0-9._-]/gi, "-")}:${imageRef.tag}`;
  const containerName = `vmsan-export-${Date.now()}`;
  const tmpTar = join(cacheDir, "rootfs.tar");

  mkdirSync(cacheDir, { recursive: true });

  try {
    consola.start(`Building image from ${imageRef.full}...`);
    const dockerfile = generateDockerfile(imageRef.full);
    execSync(`docker build -t "${buildTag}" -f - . <<'DOCKERFILE'\n${dockerfile}\nDOCKERFILE`, {
      stdio: "pipe",
      shell: "/bin/bash",
    });

    consola.start("Exporting filesystem...");
    execSync(`docker create --name "${containerName}" "${buildTag}"`, { stdio: "pipe" });
    execSync(`docker export "${containerName}" -o "${tmpTar}"`, { stdio: "pipe" });

    consola.start("Converting to ext4...");
    const tarSizeOutput = execSync(`stat -c %s "${tmpTar}"`, { encoding: "utf-8" }).trim();
    const tarBytes = Number(tarSizeOutput);
    const tarMb = tarBytes / 1024 / 1024;
    const imageSizeMb = Math.max(1024, Math.ceil(tarMb + 512));

    execSync(`dd if=/dev/zero of="${ext4Path}" bs=1M count=${imageSizeMb} 2>/dev/null`, {
      stdio: "pipe",
    });
    execSync(`mkfs.ext4 -q "${ext4Path}"`, { stdio: "pipe" });
    execSync(`tune2fs -m 0 "${ext4Path}"`, { stdio: "pipe" });

    const tmpMount = join(cacheDir, "mnt");
    mkdirSync(tmpMount, { recursive: true });
    execSync(`mount -o loop "${ext4Path}" "${tmpMount}"`, { stdio: "pipe" });

    try {
      execSync(`tar -xf "${tmpTar}" -C "${tmpMount}"`, { stdio: "pipe" });
    } finally {
      execSync(`umount "${tmpMount}"`, { stdio: "pipe" });
      execSync(`rm -rf "${tmpMount}"`, { stdio: "pipe" });
    }

    writeFileSync(
      join(cacheDir, "metadata.json"),
      JSON.stringify({ image: imageRef.full, builtAt: new Date().toISOString() }, null, 2),
    );

    consola.success(`Rootfs built from ${imageRef.full} (${imageSizeMb} MB)`);
    return ext4Path;
  } finally {
    try {
      execSync(`docker rm -f "${containerName}" 2>/dev/null`, { stdio: "pipe" });
    } catch {
      // Container may already be removed
    }
    try {
      execSync(`rm -f "${tmpTar}"`, { stdio: "pipe" });
    } catch {
      // Temp file may already be cleaned
    }
  }
}

export function resolveImageRootfs(imageRef: ImageReference, registryDir: string): string {
  const cacheDir = join(registryDir, imageRef.cacheKey);
  const ext4Path = join(cacheDir, "rootfs.ext4");

  if (existsSync(ext4Path)) {
    consola.info(`Using cached rootfs for ${imageRef.full}`);
    return ext4Path;
  }

  return buildImageRootfs(imageRef, cacheDir);
}
