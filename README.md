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
- 🛡️ **Defense-in-depth egress filtering** — DNS + SNI + L3/L4 (since 0.4.0)
- 🖥️ **Interactive shell** — WebSocket PTY with full terminal support
- 📂 **File transfer** — upload and download without SSH
- 🐳 **Docker images** — build rootfs from any OCI image with `--from-image`
- 🏃 **Command execution** — run commands with streaming output, env injection, and sudo
- 🧩 **Multiple runtimes** — `base`, `node22`, `node24`, `python3.13`
- 📸 **VM snapshots** — save and restore VM state
- 🚀 **Project deployment** — define services in `vmsan.toml`, deploy with `vmsan up`
- 🗄️ **Built-in accessories** — auto-provision postgres and redis from pre-built rootfs
- 🔑 **Encrypted secrets** — manage secrets with `vmsan secrets set/list/unset`
- 🌐 **Multi-host** — server/agent architecture for running VMs across hosts
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

### Project deployment (vmsan.toml)

```bash
# Initialize a project (creates vmsan.toml)
vmsan init

# Deploy all services
vmsan up

# Check service status
vmsan status

# Re-deploy code without recreating VMs
vmsan deploy

# Stop all services (--destroy removes everything)
vmsan down
```

### Single VM management

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

# Snapshot a running VM
vmsan snapshot create <vm-id>

# List snapshots
vmsan snapshot list

# Restore a VM from a snapshot
vmsan create --snapshot <snapshot-id>

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
| `init`     |       | Initialize a project with vmsan.toml  |
| `up`       |       | Deploy services defined in vmsan.toml |
| `deploy`   |       | Re-deploy services without recreating VMs |
| `status`   |       | Show project service status overview  |
| `down`     |       | Stop all services for the current project |
| `secrets`  |       | Manage project secrets (set, list, unset) |
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
| `logs`     |       | View VM logs (`--dns` for DNS query logs) |
| `snapshot` |       | Manage VM snapshots (create, list, delete) |
| `hosts`    |       | Manage remote hosts (add, list, remove)            |
| `server`   |       | Start the control plane server                     |
| `agent`    |       | Join a worker node to the control plane             |
| `migrate`  |       | Migrate state from JSON to SQLite                  |
| `doctor`   |       | Check system prerequisites and installation health |

## 🚀 Project Deployment

Define your stack in `vmsan.toml` and deploy with a single command.

### Example: multi-service stack

```toml
[services.web]
runtime = "node22"
build = "npm install && npm run build"
start = "npm start"
depends_on = ["postgres", "redis"]

[services.web.health_check]
type = "http"
path = "/health"
port = 3000

[accessories.postgres]
type = "postgres"

[accessories.redis]
type = "redis"

[deploy]
release = "npx prisma migrate deploy"
```

```bash
vmsan init          # creates vmsan.toml
vmsan up       # provisions VMs, builds, and starts services
vmsan status   # shows service health table
vmsan deploy   # re-deploys code (skips accessory recreation)
vmsan down     # stops everything (--destroy removes VMs)
```

### Built-in accessories

Use `type = "postgres"` or `type = "redis"` in the `[accessories]` section to auto-provision databases from pre-built rootfs images. Connection environment variables are injected automatically into dependent services.

### Secrets

```bash
vmsan secrets set DATABASE_URL=postgres://...
vmsan secrets list
vmsan secrets unset DATABASE_URL
```

Secrets are encrypted at rest and injected into service VMs at deploy time.

### Multi-Host

Run VMs across multiple hosts with a server/agent architecture:

**On the control node (server):**
```bash
vmsan server                  # Start control plane on :6443
vmsan hosts add worker-1      # Generate join command for a worker
```

**On the worker node:**
```bash
vmsan agent join --server http://10.88.0.1:6443 --token <TOKEN>
```

**Deploy to a specific host:**
```bash
vmsan create --name my-vm --host worker-1 --runtime base --vcpus 2 --memory 1024
vmsan hosts list              # Show all hosts and their VMs
```

## ⚠️ Known Limitations

- No WireGuard mesh yet — cross-host VM communication requires manual networking (0.9.0)
- No scheduler — manual host assignment with `--host` flag (0.9.0)
- Pre-built rootfs layers are large (~500 MB postgres, ~200 MB redis)
- Blue-green deploys use 2x memory during transition
- Uses `nftables` with atomic rule application for network isolation (since 0.2.0)
- Defense-in-depth egress filtering: DNS + SNI + L3/L4 (since 0.4.0)
- Per-VM dnsproxy sidecar uses ~15-25 MB RSS each (in-process DNS planned for 0.11.0)
- TLS without SNI is denied by default (no escape hatch yet)
- Domain fronting via shared CDN IPs is an accepted risk
- ICMP blocked by default from VMs (use `--allow-icmp` to enable)
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
  state.db      SQLite state store (since 0.8.0)
  vms/          VM state files (legacy JSON, migrated with `vmsan migrate`)
  jailer/       Chroot directories
  bin/          Agent binary + host binaries
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
