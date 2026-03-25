---
title: Home
seo:
  title: vmsan - Firecracker microVM Platform
  description: Spin up isolated Firecracker microVMs in seconds. Declarative deployments, mesh networking, multi-host scaling, and privilege-separated architecture.
---

::u-page-hero
---
badge:
  label: Open source on GitHub
  to: https://github.com/angelorc/vmsan
  icon: i-simple-icons-github
  color: neutral
  variant: subtle
orientation: horizontal
---
#title
vmsan

#description
Firecracker made simple. Declare your services in `vmsan.toml`, deploy with `vmsan up`, and scale across hosts with a single control plane.

#headline
  :::u-button
  ---
  size: sm
  to: /api/cli-reference
  variant: outline
  ---
  Browse CLI reference →
  :::

#links
  :::u-button
  ---
  color: primary
  size: xl
  to: /getting-started/introduction
  trailing-icon: i-lucide-arrow-right
  ---
  Get Started
  :::

  :::u-button
  ---
  color: neutral
  size: xl
  to: https://github.com/angelorc/vmsan
  target: _blank
  variant: outline
  icon: i-simple-icons-github
  ---
  Star on GitHub
  :::

#default
  ```bash [Terminal]
  curl -fsSL https://vmsan.dev/install | bash
  ```
::

::u-page-section
---
title: Watch it in action
---
<img src="https://raw.githubusercontent.com/angelorc/vmsan/main/assets/demo.gif" alt="vmsan demo" style="width: 100%; border-radius: 12px; box-shadow: 0 8px 32px rgba(0,0,0,0.2);" />
::

::u-page-section
---
title: One command is all it takes
---
  :::u-page-grid
  ---
  class: lg:grid-cols-2
  ---
    ::::u-page-card
    ---
    spotlight: true
    title: Install vmsan
    description: Install the platform in seconds. Go binaries, Firecracker, kernel, runtimes — everything you need.
    ---
    ```bash [Terminal]
    curl -fsSL https://vmsan.dev/install | bash
    ✔ vmsan installed successfully
    vmsan doctor
    ✔ All checks passed
    ```
    ::::

    ::::u-page-card
    ---
    spotlight: true
    title: Define and deploy
    description: Declare your services in vmsan.toml and bring them up with a single command.
    ---
    ```bash [Terminal]
    vmsan init
    # edit vmsan.toml
    vmsan up
    ✔ api running at 198.19.1.2
    ✔ worker running at 198.19.2.2
    ```
    ::::
  :::
::

::u-page-section
---
title: Everything you need for secure workloads.
description: vmsan wraps Firecracker with a privilege-separated architecture, declarative config, and mesh networking. No boilerplate. No complex infrastructure.
---
#features
  :::u-page-feature
  ---
  icon: i-lucide-zap
  title: Millisecond boot times
  description: Firecracker microVMs boot in a fraction of a second. Minimal memory overhead for maximum efficiency.
  to: /guide/vm-lifecycle
  ---
  :::

  :::u-page-feature
  ---
  icon: i-lucide-shield-check
  title: Hardened by default
  description: Privilege-separated gateway, jailer chroot, seccomp-bpf filters, defense-in-depth egress filtering, and cgroups. Enterprise-grade security out of the box.
  to: /guide/networking
  ---
  :::

  :::u-page-feature
  ---
  icon: i-lucide-file-code
  title: Declarative deployments
  description: Define services, resources, secrets, and network policies in vmsan.toml. Deploy everything with vmsan up.
  to: /guide/project-config
  ---
  :::

  :::u-page-feature
  ---
  icon: i-lucide-waypoints
  title: Mesh networking
  description: VMs discover each other by name with automatic DNS and routing. Build multi-service architectures without manual IP management.
  to: /guide/mesh-networking
  ---
  :::

  :::u-page-feature
  ---
  icon: i-lucide-server
  title: Multi-host ready
  description: Scale across bare-metal servers with a control plane. Register hosts, schedule VMs, and manage fleets from one CLI.
  to: /guide/multi-host
  ---
  :::

  :::u-page-feature
  ---
  icon: i-lucide-terminal
  title: Native interactive shell
  description: Connect directly to running VMs via a seamless WebSocket PTY terminal. Stream logs in real time. Leave SSH behind.
  to: /guide/vm-lifecycle#connect-to-a-running-vm
  ---
  :::

  :::u-page-feature
  ---
  icon: i-lucide-layers
  title: Multi-runtime ready
  description: Start instantly with optimized Node or Python runtimes, or bring your own custom Docker image.
  to: /guide/docker-images
  ---
  :::

  :::u-page-feature
  ---
  icon: i-lucide-cpu
  title: API-first design
  description: Parse data cleanly with built-in --json flags. Designed specifically for CI/CD, scripting, and automation.
  to: /api/cli-reference
  ---
  :::
::

::u-page-section
---
title: See it in action
description: Powerful isolation primitives wrapped in an intuitive developer experience.
---
  :::u-page-grid
  ---
  class: lg:grid-cols-2
  ---
    ::::u-page-card
    ---
    spotlight: true
    to: /guide/networking
    icon: i-lucide-network
    title: Granular network control
    description: Enforce strict allow-all or deny-all rules. Route traffic via custom domain and CIDR policies. Defense-in-depth filtering across DNS, SNI, and nftables layers.
    ---
    ```bash [Network Isolation]
    vmsan create \
        --network-policy custom \
        --allowed-domain "*.github.com" \
        --denied-cidr "10.0.0.0/8" \
        --bandwidth 50mbit
    ```
    ::::

    ::::u-page-card
    ---
    spotlight: true
    to: /guide/docker-images
    icon: i-simple-icons-docker
    title: Any container as a VM
    description: Run any Docker or OCI image natively. Boot it as a secure Firecracker microVM. Cached locally for instant reuse.
    ---
    ```bash [Docker Support]
    vmsan create --from-image python:3.13-slim
    vmsan create --from-image node:22-alpine
    vmsan create --from-image myorg/app:latest
    ```
    ::::

    ::::u-page-card
    ---
    spotlight: true
    to: /guide/vm-lifecycle#connect-to-a-running-vm
    icon: i-lucide-terminal-square
    title: Shell, logs, and file sync
    description: Access VMs with a full PTY terminal. Stream logs in real time. Upload and download files effortlessly. No SSH keys required.
    ---
    ```bash [Shell & Logs]
    vmsan connect vm-a3f7b2c
    vmsan logs vm-a3f7b2c --follow
    vmsan upload vm-a3f7b2c ./app.js /app/
    ```
    ::::

    ::::u-page-card
    ---
    spotlight: true
    to: /guide/project-config
    icon: i-lucide-file-code
    title: Declarative deploy
    description: Define your entire stack in vmsan.toml. One command to bring it all up, one command to tear it down.
    ---
    ```toml [vmsan.toml]
    [services.api]
    runtime = "node22"
    memory = 512
    command = "node server.js"

    [services.worker]
    runtime = "python3.13"
    command = "python worker.py"
    ```
    ::::
  :::
::

::u-page-section
  :::u-page-c-t-a
  ---
  variant: subtle
  title: Ready to build secure workloads?
  description: Download vmsan today. Spin up your first Firecracker microVM in under a second.
  ---
  #links
    ::::u-button
    ---
    color: primary
    size: xl
    to: /getting-started/introduction
    trailing-icon: i-lucide-arrow-right
    ---
    Read the documentation
    ::::

    ::::u-button
    ---
    color: neutral
    size: xl
    to: https://github.com/angelorc/vmsan
    target: _blank
    variant: outline
    icon: i-simple-icons-github
    ---
    Star on GitHub
    ::::
  :::
::
