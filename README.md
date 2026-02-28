# vmsan

<!-- automd:badges color="yellow" license licenseSrc bundlephobia packagephobia -->

[![npm version](https://img.shields.io/npm/v/vmsan?color=yellow)](https://npmjs.com/package/vmsan)
[![npm downloads](https://img.shields.io/npm/dm/vmsan?color=yellow)](https://npm.chart.dev/vmsan)
[![bundle size](https://img.shields.io/bundlephobia/minzip/vmsan?color=yellow)](https://bundlephobia.com/package/vmsan)
[![install size](https://badgen.net/packagephobia/install/vmsan?color=yellow)](https://packagephobia.com/result?p=vmsan)
[![license](https://img.shields.io/github/license/angelorc/vmsan?color=yellow)](https://github.com/angelorc/vmsan/blob/main/LICENSE)

<!-- /automd -->

Firecracker microVM sandbox toolkit. Create, manage, and connect to isolated Firecracker microVMs from the command line.

## Features

- Full VM lifecycle management (create, start, stop, remove)
- Network isolation with policy-based controls (allow-all, deny-all, custom domain/CIDR allowlists)
- Interactive shell access via WebSocket PTY
- File upload/download to running VMs
- Command execution with streaming output
- Multiple runtimes: `base`, `node22`, `python3.13`
- Docker image support via `--from-image`
- VM snapshots
- Structured JSON output for scripting

## Prerequisites

- Linux (x86_64 or aarch64) with KVM support
- [Bun](https://bun.sh) >= 1.2
- [Go](https://go.dev) >= 1.22 (to build the in-VM agent)
- Root/sudo access (required for TAP device networking and jailer)
- `squashfs-tools` (for rootfs conversion during install)

## Install

### 1. Install Firecracker, kernel, and rootfs

```bash
curl -fsSL https://raw.githubusercontent.com/angelorc/vmsan/main/install.sh | bash
```

To uninstall:

```bash
curl -fsSL https://raw.githubusercontent.com/angelorc/vmsan/main/install.sh | bash -s -- --uninstall
```

This downloads and installs into `~/.vmsan/`:

- Firecracker + Jailer (latest release)
- Linux kernel (vmlinux 6.1)
- Ubuntu 24.04 rootfs (converted from squashfs to ext4)

### 2. Install vmsan CLI

<!-- automd:pm-install -->

```sh
# ✨ Auto-detect
npx nypm install vmsan

# npm
npm install vmsan

# yarn
yarn add vmsan

# pnpm
pnpm add vmsan

# bun
bun install vmsan

# deno
deno install npm:vmsan
```

<!-- /automd -->

### 3. Build the in-VM agent

```bash
cd agent
make install
cd ..
```

### Link globally (optional)

```bash
bun link
```

This makes the `vmsan` command available system-wide.

## Usage

```bash
# Create and start a VM
vmsan create --runtime node22 --memory 512 --cpus 2

# Create a VM from a Docker image
vmsan create --from-image node:22-alpine

# List all VMs
vmsan list

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

```
--json       Output structured JSON
--verbose    Show detailed debug output
```

### Commands

| Command    | Alias | Description                       |
| ---------- | ----- | --------------------------------- |
| `create`   |       | Create and start a new microVM    |
| `list`     | `ls`  | List all VMs                      |
| `start`    |       | Start a stopped VM                |
| `stop`     |       | Stop a running VM                 |
| `remove`   | `rm`  | Remove a VM                       |
| `connect`  |       | Open an interactive shell to a VM |
| `upload`   |       | Upload files to a VM              |
| `download` |       | Download files from a VM          |

## Development

To use your local build instead of the installed one, link it:

```bash
bun run build
ln -sf "$(pwd)/dist/bin/cli.mjs" ~/.vmsan/bin/vmsan
```

```bash
# Dev mode (watch)
bun run dev

# Run tests
bun run test

# Type check
bun run typecheck

# Lint
bun run lint

# Format
bun run fmt
```

## Project structure

```
bin/            CLI entry point
src/
  commands/     CLI subcommands
  services/     Firecracker client, agent client, VM service
  lib/          Utilities (jailer, networking, shell, logging)
  errors/       Typed error system
  generated/    Firecracker API type definitions
agent/          Go agent that runs inside the VM
```

## How it works

1. **vmsan** uses [Firecracker](https://github.com/firecracker-microvm/firecracker) to create lightweight microVMs with a jailer for security isolation
2. Each VM gets a TAP network device with its own `/30` subnet (`172.16.{slot}.0/30`)
3. A Go-based **agent** runs inside the VM, exposing an HTTP API on port 9119 for command execution, file operations, and shell access
4. The CLI communicates with the agent over the host-guest network to manage the VM

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

## License

[MIT](./LICENSE)

<!-- automd:contributors author="angelorc" license="MIT" -->

Published under the [MIT](https://github.com/angelorc/vmsan/blob/main/LICENSE) license.
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
