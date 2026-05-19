---
name: vmsan
description: Complete reference for vmsan CLI - Firecracker microVM sandbox toolkit. Use when user needs to create, manage, connect to, or work with Firecracker microVMs. Triggers include requests to "create a microVM", "start a sandbox", "connect to VM", "run command in VM", "upload file to VM", "manage Firecracker", "snapshot VM", "isolate environment", or any task requiring lightweight virtualization. Supports millisecond boot times, security isolation, file transfer, command execution, multiple runtimes (Node.js, Python), VM snapshots, and network policies.
allowed-tools: Bash
---

# vmsan

🔥 Firecracker microVM sandbox toolkit - Create, manage, and connect to isolated Firecracker microVMs from the command line.

**Official Documentation:** https://vmsan.dev
**Version:** 0.3.x (current as of 2026)
**License:** Open Source

## Overview

vmsan is a CLI tool for working with [Firecracker microVMs](https://github.com/firecracker-microvm/firecracker) - ultra-lightweight virtual machines that boot in milliseconds and provide strong security isolation. Perfect for sandboxed execution, testing, CI/CD, development environments, and running untrusted code safely.

### Key Features

- ⚡ **Millisecond boot** — Firecracker microVMs start in < 200ms
- 🔒 **Security isolation** — jailer, seccomp, cgroups, per-VM network namespaces
- 🖥️ **Interactive shell** — WebSocket PTY with full terminal support
- 📂 **File transfer** — upload and download without SSH
- 🐳 **Docker images** — build rootfs from any OCI image with `--from-image`
- 🏃 **Command execution** — run commands with streaming output, env injection, sudo
- 🧩 **Multiple runtimes** — `base`, `node22`, `node24`, `python3.13`
- 📸 **VM snapshots** — save and restore VM state
- 📊 **JSON output** — `--json` flag for scripting and automation
- 🌐 **Network policies** — control VM internet access with allow/deny rules

## Prerequisites

### System Requirements

- **Operating System:** Linux (x86_64 or aarch64)
- **KVM Support:** Required for hardware virtualization
- **Root/sudo access:** Required for TAP device networking and jailer
- **Docker:** Required for source installs, local runtime rebuilds, and `--from-image` feature

### Check KVM Support

```bash
# Verify KVM is available
ls -la /dev/kvm

# Check if KVM module is loaded
lsmod | grep kvm

# Test KVM support
[ -w /dev/kvm ] && echo "KVM is available" || echo "KVM not available"
```

### Primary Platform

- Ubuntu 24.04 LTS (recommended)
- Other Linux distributions supported but may require additional configuration

## Installation

### Quick Install

```bash
# Install vmsan (downloads to ~/.vmsan/)
curl -fsSL https://vmsan.dev/install | bash

# Add to PATH (add to ~/.bashrc or ~/.zshrc)
export PATH="$HOME/.vmsan/bin:$PATH"

# Verify installation
vmsan --version
```

### What Gets Installed

The installer downloads and sets up:
- Firecracker + Jailer (latest release)
- Linux kernel (vmlinux 6.1)
- Ubuntu 24.04 rootfs (ext4 format)
- vmsan CLI + in-VM agent
- Runtime images (`node22`, `node24`, `python3.13`) as prebuilt artifacts

All files are installed to `~/.vmsan/` directory.

### Uninstall

```bash
curl -fsSL https://vmsan.dev/install | bash -s -- --uninstall
```

### Development Setup (Build from Source)

```bash
# Clone repository
git clone https://github.com/angelorc/vmsan.git
cd vmsan

# Install dependencies
bun install

# Build in-VM agent
cd agent && make install && cd ..

# Build CLI
bun run build

# Link local build
mkdir -p ~/.vmsan/bin
ln -sf "$(pwd)/dist/bin/cli.mjs" ~/.vmsan/bin/vmsan
```

### System Health Check

```bash
# Verify installation and prerequisites
vmsan doctor

# This checks:
# - KVM availability
# - Firecracker binary
# - Linux kernel
# - Rootfs images
# - Required permissions
# - Network configuration
```

## CLI Structure

```
vmsan
├── create              Create and start a new microVM
├── list (ls)           List all VMs
├── start               Start a stopped VM
├── stop                Stop a running VM
├── remove (rm)         Remove a VM
├── exec                Execute a command inside a running VM
├── connect             Open an interactive shell to a VM
├── upload              Upload files to a VM
├── download            Download files from a VM
├── network             Update network policy on a running VM
├── snapshot            Manage VM snapshots
│   ├── create          Create a snapshot
│   ├── list            List snapshots
│   └── delete          Delete a snapshot
└── doctor              Check system prerequisites and health
```

## Global Flags

| Flag        | Description                           |
|-------------|---------------------------------------|
| `--json`    | Output structured JSON                |
| `--verbose` | Show detailed debug output            |
| `--help`    | Show help for command                 |
| `--version` | Show vmsan version                    |

## Commands Reference

### vmsan create

Create and start a new Firecracker microVM.

#### Basic Usage

```bash
# Create VM with default settings (base runtime, 512MB RAM, 2 CPUs)
vmsan create

# Create with specific runtime
vmsan create --runtime node22
vmsan create --runtime node24
vmsan create --runtime python3.13
vmsan create --runtime base

# Create with custom resources
vmsan create --memory 1024 --cpus 4

# Create from Docker image
vmsan create --from-image node:22-alpine
vmsan create --from-image python:3.13-slim
vmsan create --from-image ubuntu:24.04

# Create from snapshot
vmsan create --snapshot <snapshot-id>

# JSON output (returns VM ID and metadata)
vmsan create --runtime node22 --json
```

#### Flags

| Flag                        | Description                                    |
|-----------------------------|------------------------------------------------|
| `--runtime RUNTIME`         | Runtime image: base, node22, node24, python3.13 |
| `--memory MB`               | Memory in MB (default: 512)                    |
| `--cpus COUNT`              | Number of CPUs (default: 2)                    |
| `--from-image IMAGE`        | Create rootfs from Docker image                |
| `--snapshot SNAPSHOT_ID`    | Create VM from snapshot                        |
| `--json`                    | Output JSON with VM details                    |

#### Examples

```bash
# Development environment with Node.js
vmsan create --runtime node22 --memory 1024 --cpus 2

# Python testing environment
vmsan create --runtime python3.13 --memory 512

# Custom environment from Docker image
vmsan create --from-image node:22-alpine --memory 2048

# Minimal base environment
vmsan create --runtime base --memory 256 --cpus 1

# High-resource VM
vmsan create --runtime node24 --memory 4096 --cpus 8
```

### vmsan list (vmsan ls)

List all microVMs managed by vmsan.

#### Usage

```bash
# List all VMs
vmsan list
vmsan ls

# JSON output
vmsan list --json

# Verbose output
vmsan list --verbose
```

#### Output Format

```
ID              STATUS      RUNTIME     MEMORY    CPUS    CREATED
vm-abc123def    running     node22      512MB     2       2m ago
vm-xyz789ghi    stopped     python3.13  1024MB    4       1h ago
```

#### JSON Output

```bash
vmsan list --json
```

```json
[
  {
    "id": "vm-abc123def",
    "status": "running",
    "runtime": "node22",
    "memory": 512,
    "cpus": 2,
    "created": "2026-01-15T10:30:00Z",
    "pid": 12345,
    "network": {
      "ip": "172.16.0.2",
      "gateway": "172.16.0.1"
    }
  }
]
```

### vmsan start

Start a stopped microVM.

#### Usage

```bash
# Start a specific VM
vmsan start <vm-id>

# With verbose output
vmsan start <vm-id> --verbose

# JSON output
vmsan start <vm-id> --json
```

#### Examples

```bash
# Start VM by ID
vmsan start vm-abc123def

# Start and verify
vmsan start vm-abc123def && vmsan list
```

### vmsan stop

Stop a running microVM.

#### Usage

```bash
# Stop a specific VM
vmsan stop <vm-id>

# Force stop (SIGKILL instead of graceful shutdown)
vmsan stop <vm-id> --force

# JSON output
vmsan stop <vm-id> --json
```

#### Examples

```bash
# Graceful stop
vmsan stop vm-abc123def

# Force stop unresponsive VM
vmsan stop vm-abc123def --force

# Stop multiple VMs
for vm in $(vmsan list --json | jq -r '.[].id'); do
  vmsan stop "$vm"
done
```

### vmsan remove (vmsan rm)

Remove a microVM (must be stopped first).

#### Usage

```bash
# Remove a VM
vmsan remove <vm-id>
vmsan rm <vm-id>

# Remove without confirmation prompt
vmsan remove <vm-id> --yes

# Force remove (stop and remove)
vmsan remove <vm-id> --force

# JSON output
vmsan remove <vm-id> --json
```

#### Examples

```bash
# Stop and remove VM
vmsan stop vm-abc123def && vmsan remove vm-abc123def

# Force remove running VM
vmsan remove vm-abc123def --force

# Remove with confirmation
vmsan rm vm-abc123def --yes

# Clean up all stopped VMs
vmsan list --json | jq -r '.[] | select(.status=="stopped") | .id' | \
  xargs -I {} vmsan remove {} --yes
```

### vmsan exec

Execute a command inside a running microVM.

#### Usage

```bash
# Execute command
vmsan exec <vm-id> <command>

# Interactive exec with PTY
vmsan exec -i <vm-id> <command>

# Run as specific user
vmsan exec --user <username> <vm-id> <command>

# Set working directory
vmsan exec --workdir /path <vm-id> <command>

# Inject environment variables
vmsan exec --env KEY=value <vm-id> <command>

# Run with sudo/root
vmsan exec --sudo <vm-id> <command>

# JSON output (includes exit code)
vmsan exec --json <vm-id> <command>
```

#### Flags

| Flag                  | Description                            |
|-----------------------|----------------------------------------|
| `-i, --interactive`   | Interactive exec with PTY              |
| `--user USERNAME`     | Run as specific user                   |
| `--workdir PATH`      | Set working directory                  |
| `--env KEY=VALUE`     | Set environment variable (repeatable)  |
| `--sudo`              | Run with sudo/root privileges          |
| `--json`              | Output JSON with exit code and output  |

#### Examples

```bash
# Simple command
vmsan exec vm-abc123def ls -la

# Interactive bash session
vmsan exec -i vm-abc123def bash

# Run Node.js script
vmsan exec vm-abc123def node --version
vmsan exec vm-abc123def node script.js

# Python script with arguments
vmsan exec vm-abc123def python3 -c "print('Hello from VM')"

# Install packages (with sudo)
vmsan exec --sudo vm-abc123def apt update
vmsan exec --sudo vm-abc123def apt install -y curl

# Set environment and working directory
vmsan exec --workdir /app --env NODE_ENV=production \
  vm-abc123def npm start

# Multiple environment variables
vmsan exec --env API_KEY=secret --env DEBUG=true \
  vm-abc123def node server.js

# Capture output
OUTPUT=$(vmsan exec vm-abc123def echo "test")

# Check exit code
vmsan exec --json vm-abc123def false | jq '.exitCode'
```

### vmsan connect

Open an interactive shell to a running microVM.

#### Usage

```bash
# Connect to VM
vmsan connect <vm-id>

# Connect as specific user
vmsan connect --user <username> <vm-id>

# Connect with specific shell
vmsan connect --shell /bin/zsh <vm-id>
```

#### Flags

| Flag                | Description                |
|---------------------|----------------------------|
| `--user USERNAME`   | Connect as specific user   |
| `--shell SHELL`     | Use specific shell         |

#### Examples

```bash
# Default connection (bash as current user)
vmsan connect vm-abc123def

# Connect as root
vmsan connect --user root vm-abc123def

# Use zsh instead of bash
vmsan connect --shell /bin/zsh vm-abc123def

# Connect and run commands
vmsan connect vm-abc123def
# Inside VM:
$ whoami
$ pwd
$ ls -la
$ exit
```

### vmsan upload

Upload files to a microVM.

#### Usage

```bash
# Upload file
vmsan upload <vm-id> <local-path> <remote-path>

# Upload directory (recursive)
vmsan upload --recursive <vm-id> <local-dir> <remote-dir>

# Set permissions after upload
vmsan upload --mode 0755 <vm-id> script.sh /usr/local/bin/script.sh

# JSON output
vmsan upload --json <vm-id> file.txt /remote/file.txt
```

#### Flags

| Flag                    | Description                       |
|-------------------------|-----------------------------------|
| `--recursive, -r`       | Upload directory recursively      |
| `--mode OCTAL`          | Set file permissions (e.g., 0755) |
| `--json`                | Output JSON                       |

#### Examples

```bash
# Upload single file
vmsan upload vm-abc123def ./app.js /home/user/app.js

# Upload configuration file
vmsan upload vm-abc123def ./config.json /etc/myapp/config.json

# Upload executable script
vmsan upload --mode 0755 vm-abc123def ./deploy.sh /usr/local/bin/deploy.sh

# Upload entire directory
vmsan upload --recursive vm-abc123def ./dist /var/www/html

# Upload and verify
vmsan upload vm-abc123def file.txt /tmp/file.txt && \
  vmsan exec vm-abc123def cat /tmp/file.txt

# Upload Node.js project
vmsan upload --recursive vm-abc123def ./my-app /app
vmsan exec --workdir /app vm-abc123def npm install
```

### vmsan download

Download files from a microVM.

#### Usage

```bash
# Download file
vmsan download <vm-id> <remote-path> <local-path>

# Download directory (recursive)
vmsan download --recursive <vm-id> <remote-dir> <local-dir>

# JSON output
vmsan download --json <vm-id> /remote/file.txt ./file.txt
```

#### Flags

| Flag                | Description                  |
|---------------------|------------------------------|
| `--recursive, -r`   | Download directory recursively |
| `--json`            | Output JSON                  |

#### Examples

```bash
# Download single file
vmsan download vm-abc123def /var/log/app.log ./app.log

# Download build artifacts
vmsan download vm-abc123def /app/dist/bundle.js ./bundle.js

# Download entire directory
vmsan download --recursive vm-abc123def /var/www/html ./backup

# Download logs
vmsan download vm-abc123def /var/log/syslog ./vm-syslog.log

# Download and process
vmsan download vm-abc123def /tmp/results.json ./results.json && \
  jq . ./results.json

# Backup VM data
vmsan download --recursive vm-abc123def /home/user ./vm-backup
```

### vmsan network

Update network policy on a running microVM.

#### Usage

```bash
# Update network policy
vmsan network <vm-id> <policy>

# Allow all outbound connections
vmsan network <vm-id> allow-all

# Deny all outbound connections (air-gapped)
vmsan network <vm-id> deny-all

# Allow specific domains/IPs
vmsan network <vm-id> allow-list --domains example.com,api.github.com
vmsan network <vm-id> allow-list --ips 1.1.1.1,8.8.8.8

# Deny specific domains/IPs
vmsan network <vm-id> deny-list --domains malicious.com

# JSON output
vmsan network --json <vm-id> allow-all
```

#### Policies

| Policy         | Description                                      |
|----------------|--------------------------------------------------|
| `allow-all`    | Allow all outbound connections (default)         |
| `deny-all`     | Block all outbound connections (air-gapped)      |
| `allow-list`   | Allow only specified domains/IPs                 |
| `deny-list`    | Block specified domains/IPs                      |

#### Flags

| Flag                      | Description                           |
|---------------------------|---------------------------------------|
| `--domains DOMAINS`       | Comma-separated domain list           |
| `--ips IPS`               | Comma-separated IP address list       |
| `--json`                  | Output JSON                           |

#### Examples

```bash
# Isolate VM from network
vmsan network vm-abc123def deny-all

# Allow only specific APIs
vmsan network vm-abc123def allow-list \
  --domains api.github.com,npmjs.org,registry.npmjs.org

# Allow specific IP ranges
vmsan network vm-abc123def allow-list \
  --ips 10.0.0.0/8,172.16.0.0/12

# Block known bad domains
vmsan network vm-abc123def deny-list \
  --domains malicious.com,spam.example.com

# Re-enable full internet access
vmsan network vm-abc123def allow-all

# Check current policy
vmsan list --json | jq '.[] | select(.id=="vm-abc123def") | .network'
```

### vmsan snapshot create

Create a snapshot of a running microVM.

#### Usage

```bash
# Create snapshot
vmsan snapshot create <vm-id>

# Create with custom name
vmsan snapshot create <vm-id> --name "before-upgrade"

# Create with description
vmsan snapshot create <vm-id> \
  --name "v1.0" \
  --description "Stable version 1.0 release"

# JSON output
vmsan snapshot create --json <vm-id>
```

#### Flags

| Flag                        | Description                    |
|-----------------------------|--------------------------------|
| `--name NAME`               | Snapshot name                  |
| `--description DESC`        | Snapshot description           |
| `--json`                    | Output JSON with snapshot ID   |

#### Examples

```bash
# Quick snapshot
vmsan snapshot create vm-abc123def

# Named snapshot before changes
vmsan snapshot create vm-abc123def --name "pre-migration"

# Snapshot with metadata
vmsan snapshot create vm-abc123def \
  --name "stable-v2.1" \
  --description "Tested and verified v2.1 release"

# Snapshot for backup
SNAPSHOT_ID=$(vmsan snapshot create --json vm-abc123def | jq -r '.id')
echo "Created snapshot: $SNAPSHOT_ID"
```

### vmsan snapshot list

List all available snapshots.

#### Usage

```bash
# List all snapshots
vmsan snapshot list

# List snapshots for specific VM
vmsan snapshot list --vm <vm-id>

# JSON output
vmsan snapshot list --json
```

#### Examples

```bash
# View all snapshots
vmsan snapshot list

# Find specific snapshot
vmsan snapshot list --json | jq '.[] | select(.name=="stable-v2.1")'

# List snapshots by VM
vmsan snapshot list --vm vm-abc123def

# Show snapshot details
vmsan snapshot list --json | jq '.[] | {id, name, created, size}'
```

### vmsan snapshot delete

Delete a snapshot.

#### Usage

```bash
# Delete snapshot
vmsan snapshot delete <snapshot-id>

# Delete without confirmation
vmsan snapshot delete <snapshot-id> --yes

# JSON output
vmsan snapshot delete --json <snapshot-id>
```

#### Examples

```bash
# Delete snapshot
vmsan snapshot delete snap-xyz789abc

# Delete with confirmation
vmsan snapshot delete snap-xyz789abc --yes

# Clean up old snapshots (keep last 3)
vmsan snapshot list --json | \
  jq -r 'sort_by(.created) | reverse | .[3:] | .[].id' | \
  xargs -I {} vmsan snapshot delete {} --yes
```

### vmsan doctor

Check system prerequisites and installation health.

#### Usage

```bash
# Run health check
vmsan doctor

# Verbose output
vmsan doctor --verbose

# JSON output
vmsan doctor --json
```

#### What It Checks

- KVM availability and permissions
- Firecracker binary installation
- Linux kernel presence
- Rootfs images
- Network configuration (TAP devices, nftables)
- Required system packages
- File permissions

#### Examples

```bash
# Basic health check
vmsan doctor

# Detailed diagnostics
vmsan doctor --verbose

# Parse results programmatically
vmsan doctor --json | jq '.checks[] | select(.status!="ok")'
```

## Common Workflows

### Quick Start: Run Node.js App in Isolated VM

```bash
# 1. Create VM with Node.js runtime
VM_ID=$(vmsan create --runtime node22 --json | jq -r '.id')

# 2. Upload application
vmsan upload --recursive "$VM_ID" ./my-app /app

# 3. Install dependencies
vmsan exec --workdir /app "$VM_ID" npm install

# 4. Run application
vmsan exec --workdir /app "$VM_ID" npm start

# 5. Clean up when done
vmsan stop "$VM_ID"
vmsan remove "$VM_ID"
```

### Run Python Script in Sandbox

```bash
# Create Python VM
VM_ID=$(vmsan create --runtime python3.13 --json | jq -r '.id')

# Upload script
vmsan upload "$VM_ID" ./script.py /tmp/script.py

# Install dependencies
vmsan exec "$VM_ID" pip install --user requests

# Run script
vmsan exec "$VM_ID" python3 /tmp/script.py

# Cleanup
vmsan stop "$VM_ID" && vmsan remove "$VM_ID"
```

### Build and Test Workflow

```bash
# Create snapshot before tests
VM_ID=$(vmsan create --runtime node22 --json | jq -r '.id')
vmsan upload --recursive "$VM_ID" ./project /app
vmsan exec --workdir /app "$VM_ID" npm install

# Create snapshot of clean state
SNAPSHOT=$(vmsan snapshot create --name "clean-install" "$VM_ID" --json | jq -r '.id')

# Run tests
vmsan exec --workdir /app "$VM_ID" npm test

# If tests fail, restore from snapshot
vmsan create --snapshot "$SNAPSHOT"
```

### CI/CD Integration

```bash
#!/bin/bash
# ci-test.sh - Run tests in isolated VM

set -e

# Create VM
echo "Creating test VM..."
VM_ID=$(vmsan create --runtime node22 --memory 2048 --json | jq -r '.id')

# Cleanup on exit
trap "vmsan stop $VM_ID && vmsan remove $VM_ID --yes" EXIT

# Upload code
echo "Uploading code..."
vmsan upload --recursive "$VM_ID" . /app

# Install deps
echo "Installing dependencies..."
vmsan exec --workdir /app "$VM_ID" npm ci

# Run tests
echo "Running tests..."
vmsan exec --workdir /app "$VM_ID" npm test

# Download coverage
vmsan download --recursive "$VM_ID" /app/coverage ./coverage

echo "Tests passed!"
```

### Air-Gapped Environment

```bash
# Create VM with no internet access
VM_ID=$(vmsan create --runtime base --json | jq -r '.id')
vmsan network "$VM_ID" deny-all

# Upload everything needed
vmsan upload --recursive "$VM_ID" ./app /app
vmsan upload --recursive "$VM_ID" ./node_modules /app/node_modules

# Run application (no outbound network)
vmsan exec --workdir /app "$VM_ID" node server.js
```

### Multi-VM Development Setup

```bash
# Create database VM
DB_VM=$(vmsan create --from-image postgres:16 --memory 1024 --json | jq -r '.id')

# Create API VM
API_VM=$(vmsan create --runtime node22 --memory 512 --json | jq -r '.id')
vmsan upload --recursive "$API_VM" ./api /app

# Create worker VM
WORKER_VM=$(vmsan create --runtime node22 --memory 512 --json | jq -r '.id')
vmsan upload --recursive "$WORKER_VM" ./worker /app

# List all VMs
vmsan list

# Cleanup all
for vm in "$DB_VM" "$API_VM" "$WORKER_VM"; do
  vmsan stop "$vm"
  vmsan remove "$vm" --yes
done
```

## JSON Output and Scripting

All commands support `--json` flag for machine-readable output.

### Parse VM List

```bash
# Get all running VMs
vmsan list --json | jq '.[] | select(.status=="running") | .id'

# Get VM IDs sorted by memory
vmsan list --json | jq 'sort_by(.memory) | .[].id'

# Find VMs by runtime
vmsan list --json | jq '.[] | select(.runtime=="node22") | {id, memory, cpus}'
```

### Automated Cleanup

```bash
# Stop and remove all stopped VMs
vmsan list --json | \
  jq -r '.[] | select(.status=="stopped") | .id' | \
  xargs -I {} vmsan remove {} --yes

# Remove VMs older than 1 hour
vmsan list --json | \
  jq -r ".[] | select(.created < \"$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)\") | .id" | \
  xargs -I {} sh -c "vmsan stop {} && vmsan remove {} --yes"
```

### Conditional Execution

```bash
# Create VM and check success
if vmsan create --runtime node22 --json | jq -e '.id' > /dev/null; then
  echo "VM created successfully"
else
  echo "Failed to create VM"
  exit 1
fi
```

## Environment Variables

| Variable              | Description                              |
|-----------------------|------------------------------------------|
| `VMSAN_HOME`          | Installation directory (default: ~/.vmsan) |
| `VMSAN_SOCKET`        | Control socket path                      |
| `VMSAN_LOG_LEVEL`     | Logging level: debug, info, warn, error  |

```bash
# Use custom installation directory
export VMSAN_HOME=/opt/vmsan

# Enable debug logging
export VMSAN_LOG_LEVEL=debug

# Run command with custom config
VMSAN_HOME=/custom/path vmsan list
```

## Known Limitations

- **No inter-VM networking** — VMs cannot communicate directly (planned for 0.4.0)
- **No declarative config** — No `vmsan.toml` configuration file yet (planned for 0.5.0)
- **No multi-host support** — Single-host only (planned for 0.7.0)
- **ICMP blocked** — Prevents ICMP tunneling (ping will not work from VMs)
- **UDP mostly blocked** — Only DNS (port 53) allowed; QUIC/HTTP3 unavailable
- **NTP blocked** — Long-running VMs may experience time drift (kvm-clock used)
- **Linux only** — Requires Linux with KVM support
- **Root required** — For TAP devices and jailer isolation

## Troubleshooting

### VM Won't Start

```bash
# Check system health
vmsan doctor

# Verify KVM access
ls -la /dev/kvm

# Check for running VMs
vmsan list

# View verbose logs
vmsan create --runtime node22 --verbose
```

### Connection Issues

```bash
# Verify VM is running
vmsan list

# Check VM logs
vmsan exec <vm-id> journalctl -n 50

# Test network connectivity
vmsan exec <vm-id> ping 8.8.8.8
vmsan exec <vm-id> curl -I https://google.com
```

### File Transfer Failures

```bash
# Check disk space in VM
vmsan exec <vm-id> df -h

# Verify file permissions
vmsan exec <vm-id> ls -la /path/to/file

# Try with verbose output
vmsan upload --verbose <vm-id> file.txt /remote/file.txt
```

### Permission Errors

```bash
# Ensure running with appropriate permissions
sudo vmsan doctor

# Check TAP device permissions
ip tuntap show

# Verify nftables rules
sudo nft list ruleset
```

## Security Considerations

1. **Isolation:** VMs run with strong isolation (jailer, seccomp, cgroups, namespaces)
2. **Network Policy:** Use `deny-all` or `allow-list` for untrusted workloads
3. **Root Access:** vmsan requires root/sudo for TAP devices and jailer
4. **Secrets:** Never commit VM IDs or snapshot IDs containing sensitive data
5. **Updates:** Keep vmsan and Firecracker updated for security patches

## Best Practices

1. **Resource Allocation:** Start with minimal resources; scale up if needed
   ```bash
   vmsan create --memory 256 --cpus 1  # Minimal
   ```

2. **Use Snapshots:** Create snapshots before risky operations
   ```bash
   vmsan snapshot create <vm-id> --name "before-upgrade"
   ```

3. **Clean Up:** Always remove VMs when done
   ```bash
   trap "vmsan stop $VM_ID && vmsan remove $VM_ID --yes" EXIT
   ```

4. **Network Isolation:** Default to restricted network policies
   ```bash
   vmsan create --runtime base
   vmsan network <vm-id> allow-list --domains api.example.com
   ```

5. **JSON Output:** Use JSON for automation and scripting
   ```bash
   VM_ID=$(vmsan create --json --runtime node22 | jq -r '.id')
   ```

6. **Error Handling:** Always check exit codes
   ```bash
   if ! vmsan exec <vm-id> test-command; then
     echo "Command failed"
     exit 1
   fi
   ```

## References

- **Official Website:** https://vmsan.dev
- **GitHub Repository:** https://github.com/angelorc/vmsan
- **Firecracker:** https://github.com/firecracker-microvm/firecracker
- **Firecracker API:** https://github.com/firecracker-microvm/firecracker/blob/main/docs/api_requests
- **Installation Guide:** https://vmsan.dev/install

## Examples Repository

For more examples and use cases, see:
- CI/CD integration templates
- Docker image conversion examples
- Network policy configurations
- Snapshot workflows
- Multi-VM orchestration

All available at: https://vmsan.dev/examples

