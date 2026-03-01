---
title: Home
seo:
  title: vmsan - Firecracker microVM Sandbox Toolkit
  description: Spin up isolated Firecracker microVMs in seconds. Full lifecycle management, network isolation, Docker image support, and interactive shell access.
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
Firecracker made simple. Spin up secure microVMs in milliseconds, from install to interactive shell in one command 
`vmsan create --connect`

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
  $ curl -fsSL https://vmsan.dev/install | bash
  ```
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
    description: Install the CLI in seconds with a single command.
    ---
    ```bash [Terminal]
    $ curl -fsSL https://vmsan.dev/install | bash
    ✔ vmsan installed successfully
    $ vmsan --version
    vmsan 0.1.0
    ```
    ::::

    ::::u-page-card
    ---
    spotlight: true
    title: Or jump straight in
    description: Add --connect to land in a shell instantly.
    ---
    ```bash [Terminal]
    $ vmsan create --connect
    ✔ VM created: vm-f91c4e0
    ✔ Connected to vm-f91c4e0
    root@vm-f91c4e0:~#
    ```
    ::::
  :::
::

::u-page-section
---
title: Everything you need for secure workloads.
description: vmsan wraps Firecracker with a batteries-included CLI. No boilerplate. No complex infrastructure. Just fast, secure VMs.
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
  description: Jailer chroot, seccomp-bpf filters, PID namespaces, and cgroups. Enterprise-grade security out of the box.
  to: /guide/networking
  ---
  :::

  :::u-page-feature
  ---
  icon: i-lucide-terminal
  title: Native interactive shell
  description: Connect directly to running VMs via a seamless WebSocket PTY terminal. Leave SSH behind.
  to: /guide/vm-lifecycle#connect-to-a-running-vm
  ---
  :::

  :::u-page-feature
  ---
  icon: i-lucide-file-up
  title: Instant file sync
  description: Push and pull files securely over the agent API. No SCP or complex folder mounting required.
  to: /guide/file-operations
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
    description: Enforce strict allow-all or deny-all rules. Route traffic via custom domain and CIDR policies. Throttle bandwidth instantly.
    ---
    ```bash [Network Isolation]
    $ vmsan create \
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
    $ vmsan create --from-image python:3.13-slim
    $ vmsan create --from-image node:22-alpine
    $ vmsan create --from-image myorg/app:latest
    ```
    ::::

    ::::u-page-card
    ---
    spotlight: true
    to: /guide/vm-lifecycle#connect-to-a-running-vm
    icon: i-lucide-terminal-square
    title: Zero-config interactive shell
    description: Access your VMs with a full WebSocket PTY terminal. Upload and download files effortlessly. No SSH keys required.
    ---
    ```bash [Shell & Files]
    $ vmsan connect vm-a3f7b2c
    $ vmsan upload vm-a3f7b2c ./app.js /app.js
    $ vmsan download vm-a3f7b2c /app.log ./
    ```
    ::::

    ::::u-page-card
    ---
    spotlight: true
    to: /api/cli-reference
    icon: i-lucide-braces
    title: Built for automation
    description: Every single command supports structured --json output. Script your infrastructure. Automate your workflows.
    ---
    ```bash [JSON Output]
    $ vmsan list --json
    [{"id":"vm-a3f7b2c","status":"running",
      "runtime":"node22","memory":128,
      "cpus":1,"ip":"172.16.1.2",
      "networkPolicy":"allow-all"}]
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
