export const createCommandArgs = {
  vcpus: {
    type: "string",
    default: "1",
    description: "Number of vCPUs (default: 1)",
  },
  memory: {
    type: "string",
    default: "128",
    description: "Memory in MiB (default: 128)",
  },
  kernel: {
    type: "string",
    description: "Path to kernel image. Auto-detected from kernels/ if not specified.",
  },
  rootfs: {
    type: "string",
    description: "Path to rootfs image. Auto-detected from rootfs/ if not specified.",
  },
  "from-image": {
    type: "string",
    description: "Build rootfs from a Docker/OCI image (e.g. ubuntu:latest).",
  },
  runtime: {
    type: "string",
    default: "base",
    description:
      "Runtime label (e.g. python3.13, node22, node22-demo). node22-demo auto-selects node:22 image and serves a branded welcome page. Default: base",
  },
  project: {
    type: "string",
    default: "",
    description: "Project label for grouping VMs",
  },
  disk: {
    type: "string",
    default: "10gb",
    description: "Root disk size in GB (default: 10gb)",
  },
  timeout: {
    type: "string",
    description: "Auto-shutdown timeout (e.g. 1h, 30m, 2h30m)",
  },
  "publish-port": {
    type: "string",
    description: "Ports to forward to the VM (comma-separated, e.g. 8080,3000)",
  },
  snapshot: {
    type: "string",
    description: "Snapshot ID to restore from",
  },
  "network-policy": {
    type: "string",
    default: "allow-all",
    description:
      "Base network mode: allow-all (default), deny-all, or custom. Auto-promoted to custom when domains or CIDRs are provided.",
  },
  "allowed-domain": {
    type: "string",
    description: "Domains/patterns to allow (comma-separated). Wildcard * for subdomains.",
  },
  "allowed-cidr": {
    type: "string",
    description: "Address ranges to allow (comma-separated CIDR, e.g. 10.0.0.0/8)",
  },
  "denied-cidr": {
    type: "string",
    description: "Address ranges to deny (comma-separated CIDR). Takes precedence over all allows.",
  },
  "no-seccomp": {
    type: "boolean",
    default: false,
    description: "Disable seccomp-bpf filter for the Firecracker process.",
  },
  "no-pid-ns": {
    type: "boolean",
    default: false,
    description: "Disable PID namespace isolation for the jailer.",
  },
  "no-cgroup": {
    type: "boolean",
    default: false,
    description: "Disable cgroup resource limits for CPU and memory.",
  },
  "no-netns": {
    type: "boolean",
    default: false,
    description: "Disable per-VM network namespace isolation.",
  },
  bandwidth: {
    type: "string",
    description: "Max bandwidth per VM (e.g., 50mbit, 100mbit). Default: unlimited.",
  },
  connect: {
    type: "boolean",
    default: false,
    description: "Automatically connect to the VM shell after creation.",
  },
  silent: {
    type: "boolean",
    default: false,
    description: "Suppress all output",
  },
} as const;
