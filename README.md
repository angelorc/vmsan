<p align="center">
  <h1 align="center">vmsan</h1>
  <p align="center">🔥 Firecracker microVM sandbox toolkit</p>
</p>

<!-- automd:badges color="yellow" license licenseSrc bundlephobia packagephobia -->

[![npm version](https://img.shields.io/npm/v/vmsan?color=yellow)](https://npmjs.com/package/vmsan)
[![npm downloads](https://img.shields.io/npm/dm/vmsan?color=yellow)](https://npm.chart.dev/vmsan)
[![bundle size](https://img.shields.io/bundlephobia/minzip/vmsan?color=yellow)](https://bundlephobia.com/package/vmsan)
[![install size](https://badgen.net/packagephobia/install/vmsan?color=yellow)](https://packagephobia.com/result?p=vmsan)
[![license](https://img.shields.io/github/license/angelorc/vmsan?color=yellow)](https://github.com/angelorc/vmsan/blob/main/LICENSE)

<!-- /automd -->

Create, manage, and connect to isolated [Firecracker](https://github.com/firecracker-microvm/firecracker) microVMs from the command line. Boot a sandboxed VM in milliseconds, run commands, transfer files — no SSH required.

> 📚 **[Read the full documentation at vmsan.dev](https://vmsan.dev)**

<p align="center">
  <img src="assets/demo.gif" alt="vmsan demo" width="720" />
</p>

## ✨ Features

- ⚡ **Millisecond boot** — Firecracker microVMs start in < 200ms
- 🔒 **Security isolation** — jailer, seccomp, cgroups, and per-VM network namespaces
- 🖥️ **Interactive shell** — WebSocket PTY with full terminal support
- 📂 **File transfer** — upload and download without SSH
- 🐳 **Docker images** — build rootfs from any OCI image with `--from-image`
- 🏃 **Command execution** — run commands with streaming output, env injection, and sudo
- 🧩 **Multiple runtimes** — `base`, `node22`, `node24`, `python3.13`
- 📸 **VM snapshots** — save and restore VM state
- 📊 **JSON output** — `--json` flag for scripting and automation

## 📋 Prerequisites

- Linux (x86_64 or aarch64) with KVM support
- Root/sudo access (required for TAP device networking and jailer)
- Docker (for source installs, local runtime rebuilds, and `--from-image`)

## 🚀 Install

```bash
curl -fsSL https://vmsan.dev/install | bash
```

This downloads and installs everything into `~/.vmsan/`:

- Firecracker + Jailer (latest release)
- Linux kernel (vmlinux 6.1)
- Ubuntu 24.04 rootfs (converted from squashfs to ext4)
- vmsan CLI + in-VM agent
- Runtime images (`node22`, `node24`, `python3.13`) downloaded as prebuilt artifacts

Standard release installs download built-in runtimes from `https://artifacts.vmsan.dev/` and do not require Docker. Source installs still build runtimes locally.

<details>
<summary>Uninstall</summary>

```bash
curl -fsSL https://vmsan.dev/install | bash -s -- --uninstall
```

</details>

<details>
<summary>Development setup</summary>

If you want to build from source:

```bash
# Install dependencies
bun install

# Build the in-VM agent
cd agent && make install && cd ..

# Build the CLI
bun run build

# Link local build
mkdir -p ~/.vmsan/bin
ln -sf "$(pwd)/dist/bin/cli.mjs" ~/.vmsan/bin/vmsan
```

</details>

## 📖 Usage

```bash
# Create and start a VM
vmsan create --runtime node22 --memory 512 --cpus 2

# Create a VM from a Docker image
vmsan create --from-image node:22-alpine

# List all VMs
vmsan list

# Execute a command inside a VM
vmsan exec <vm-id> ls -la

# Interactive exec with PTY
vmsan exec -i <vm-id> bash

# Connect to a running VM shell
vmsan connect <vm-id>

# Upload a file to a VM
vmsan upload <vm-id> ./local-file.txt /remote/path/file.txt

# Download a file from a VM
vmsan download <vm-id> /remote/path/file.txt ./local-file.txt

# Stop a VM
vmsan stop <vm-id>

# Remove a VM
vmsan remove <vm-id>
```

### Global flags

| Flag        | Description                |
| ----------- | -------------------------- |
| `--json`    | Output structured JSON     |
| `--verbose` | Show detailed debug output |

### Commands

| Command    | Alias | Description                           |
| ---------- | ----- | ------------------------------------- |
| `create`   |       | Create and start a new microVM        |
| `list`     | `ls`  | List all VMs                          |
| `start`    |       | Start a stopped VM                    |
| `stop`     |       | Stop a running VM                     |
| `remove`   | `rm`  | Remove a VM                           |
| `exec`     |       | Execute a command inside a running VM  |
| `connect`  |       | Open an interactive shell to a VM     |
| `upload`   |       | Upload files to a VM                  |
| `download` |       | Download files from a VM              |
| `network`  |       | Update network policy on a running VM |
| `doctor`   |       | Check system prerequisites and installation health |

## ⚠️ Known Limitations

- No inter-VM networking (planned for 0.4.0)
- No declarative config / `vmsan.toml` (planned for 0.5.0)
- No multi-host support (planned for 0.7.0)
- Uses `nftables` with atomic rule application for network isolation (since 0.2.0)
- No DNS or SNI filtering (planned for 0.3.0)
- ICMP blocked by default from VMs (prevents ICMP tunneling)
- UDP blocked by default except DNS (prevents UDP exfiltration; QUIC/HTTP3 will not work)
- NTP (UDP 123) blocked — kvm-clock used for time sync, long-running VMs may drift
- Host firewall (ufw/firewalld) may need explicit allow rules for vmsan traffic
- Linux only — requires KVM support
- Primary platform: Ubuntu 24.04 LTS

## 🛠️ Development

```bash
# Build
bun run build

# Link local build
ln -sf "$(pwd)/dist/bin/cli.mjs" ~/.vmsan/bin/vmsan

# Dev mode (watch)
bun run dev

# Run tests
bun run test

# Type check
bun run typecheck

# Lint & format
bun run lint
bun run fmt
```

## 🏗️ Architecture

```
bin/            CLI entry point
src/
  commands/     CLI subcommands
  services/     Firecracker client, agent client, VM service
  lib/          Utilities (jailer, networking, shell, logging)
  errors/       Typed error system
  generated/    Firecracker API type definitions
agent/          Go agent that runs inside the VM
docs/           Documentation site (vmsan.dev)
```

### How it works

1. **vmsan** uses [Firecracker](https://github.com/firecracker-microvm/firecracker) to create lightweight microVMs with a jailer for security isolation
2. Each VM gets a TAP network device with its own `/30` subnet (`198.19.{slot}.0/30`)
3. A Go-based **agent** runs inside the VM, exposing an HTTP API for command execution, file operations, and shell access
4. The CLI communicates with the agent over the host-guest network

State is persisted in `~/.vmsan/`:

```
~/.vmsan/
  vms/          VM state files (JSON)
  jailer/       Chroot directories
  bin/          Agent binary
  kernels/      VM kernel images
  rootfs/       Base root filesystems
  registry/     Docker image rootfs cache
  snapshots/    VM snapshots
```

## ⚖️ How vmsan compares

| | vmsan | Docker | gVisor | Kata Containers | Vagrant |
|---|---|---|---|---|---|
| **Isolation level** | ✅ Hardware (KVM) | ❌ Shared kernel | ⚠️ User-space kernel | ✅ Hardware (QEMU/CH) | ✅ Hardware (VBox/VMware) |
| **Boot time** | ✅ ~125ms | ✅ ~50ms | ✅ ~5ms | ⚠️ ~200ms+ | ❌ 30-60s |
| **Setup complexity** | ✅ One command | ✅ Low | ⚠️ Medium | ❌ High | ⚠️ Medium |
| **Security model** | ✅ Jailer + seccomp + cgroups + dedicated kernel | ⚠️ Namespaces + cgroups | ⚠️ Syscall filtering | ✅ Full VM + nested containers | ✅ Full VM |
| **Network isolation** | ✅ Built-in policies (allow/deny/custom) | ❌ Manual (iptables) | ⚠️ Inherits Docker | ❌ Manual | ⚠️ NAT/bridged |
| **Docker image support** | ✅ `--from-image` | ✅ Native | ✅ Via runsc | ✅ Via containerd | ❌ |
| **Interactive shell** | ✅ WebSocket PTY | ✅ exec | ✅ exec | ✅ exec | ✅ SSH |
| **File transfer** | ✅ Built-in upload/download | ✅ cp | ✅ cp | ✅ cp | ⚠️ Shared folders / SCP |
| **JSON output** | ✅ All commands | ⚠️ Partial | ❌ | ⚠️ Partial | ❌ |
| **Memory overhead** | ✅ ~5 MiB per VM | ✅ ~1 MiB | ⚠️ ~15 MiB | ❌ ~30 MiB+ | ❌ 512 MiB+ |
| **Best for** | 🏆 AI sandboxing, untrusted code, multi-tenant | General workloads | K8s hardening | K8s compliance | Dev environments |

**Why vmsan?** Docker shares the host kernel — a container escape means game over. gVisor intercepts syscalls in user-space, reducing attack surface but not eliminating it. Kata Containers provides real VM isolation but requires complex orchestration (containerd, shimv2, K8s). Vagrant boots full VMs that take 30+ seconds and hundreds of MBs.

vmsan gives you **hardware-level isolation** with Firecracker's minimal attack surface (< 50k lines of code), boots in **milliseconds**, and requires **zero configuration** — install and go.

## 📄 License

[Apache-2.0](./LICENSE)

<!-- automd:contributors author="angelorc" license="Apache-2.0" -->

Published under the [APACHE-2.0](https://github.com/angelorc/vmsan/blob/main/LICENSE) license.
Made by [@angelorc](https://github.com/angelorc) and [community](https://github.com/angelorc/vmsan/graphs/contributors) 💛
<br><br>
<a href="https://github.com/angelorc/vmsan/graphs/contributors">
<img src="https://contrib.rocks/image?repo=angelorc/vmsan" />
</a>

<!-- /automd -->

<!-- automd:with-automd -->

---

_🤖 auto updated with [automd](https://automd.unjs.io)_

<!-- /automd -->
