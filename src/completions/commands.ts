export interface FlagDef {
  long: string;
  short?: string;
  description: string;
  takesArg: boolean;
  choices?: string[];
  completeWith?: "file";
  /** Only offered at the top level, not per-subcommand */
  topLevelOnly?: boolean;
}

export interface PositionalDef {
  completeWith: "vmId" | "file" | "shell" | "none";
  variadic: boolean;
  description: string;
}

export interface SubcommandDef {
  name: string;
  description: string;
  positionals: PositionalDef[];
  flags: FlagDef[];
}

export const CLI_NAME = "vmsan";

export const GLOBAL_FLAGS: FlagDef[] = [
  { long: "json", description: "Output structured JSON", takesArg: false },
  { long: "verbose", description: "Show detailed debug output", takesArg: false },
  { long: "version", description: "Show version", takesArg: false, topLevelOnly: true },
  { long: "help", description: "Show help", takesArg: false, topLevelOnly: true },
];

export const SUBCOMMANDS: SubcommandDef[] = [
  {
    name: "create",
    description: "Create and start a Firecracker microVM",
    positionals: [],
    flags: [
      { long: "vcpus", description: "Number of vCPUs", takesArg: true },
      { long: "memory", description: "Memory in MiB", takesArg: true },
      { long: "kernel", description: "Path to kernel image", takesArg: true, completeWith: "file" },
      { long: "rootfs", description: "Path to rootfs image", takesArg: true, completeWith: "file" },
      { long: "from-image", description: "Docker/OCI image (e.g. ubuntu:latest)", takesArg: true },
      {
        long: "runtime",
        description: "Runtime environment",
        takesArg: true,
        choices: ["base", "node22", "node24", "python3.13"],
      },
      { long: "project", description: "Project label for grouping VMs", takesArg: true },
      { long: "disk", description: "Root disk size (e.g. 10gb)", takesArg: true },
      { long: "timeout", description: "Auto-shutdown timeout (e.g. 30s, 5m, 1h)", takesArg: true },
      { long: "publish-port", description: "Ports to forward (comma-separated)", takesArg: true },
      { long: "snapshot", description: "Snapshot ID to restore from", takesArg: true },
      {
        long: "network-policy",
        description: "Network mode",
        takesArg: true,
        choices: ["allow-all", "deny-all", "custom"],
      },
      { long: "allowed-domain", description: "Domains to allow (comma-separated)", takesArg: true },
      { long: "allowed-cidr", description: "CIDRs to allow (comma-separated)", takesArg: true },
      { long: "denied-cidr", description: "CIDRs to deny (comma-separated)", takesArg: true },
      { long: "no-seccomp", description: "Disable seccomp-bpf filter", takesArg: false },
      { long: "no-pid-ns", description: "Disable PID namespace isolation", takesArg: false },
      { long: "no-cgroup", description: "Disable cgroup resource limits", takesArg: false },
      { long: "no-netns", description: "Disable network namespace isolation", takesArg: false },
      { long: "bandwidth", description: "Max bandwidth (e.g. 100mbit)", takesArg: true },
      { long: "connect", description: "Connect to VM shell after creation", takesArg: false },
      { long: "silent", description: "Suppress all output", takesArg: false },
    ],
  },
  {
    name: "list",
    description: "List all VMs",
    positionals: [],
    flags: [],
  },
  {
    name: "ls",
    description: "List all VMs (alias for list)",
    positionals: [],
    flags: [],
  },
  {
    name: "start",
    description: "Start a previously stopped VM",
    positionals: [{ completeWith: "vmId", variadic: false, description: "VM ID" }],
    flags: [],
  },
  {
    name: "stop",
    description: "Stop one or more running VMs",
    positionals: [{ completeWith: "vmId", variadic: true, description: "VM ID" }],
    flags: [],
  },
  {
    name: "remove",
    description: "Remove one or more VMs",
    positionals: [{ completeWith: "vmId", variadic: true, description: "VM ID" }],
    flags: [{ long: "force", short: "f", description: "Force removal of running VMs", takesArg: false }],
  },
  {
    name: "rm",
    description: "Remove one or more VMs (alias for remove)",
    positionals: [{ completeWith: "vmId", variadic: true, description: "VM ID" }],
    flags: [{ long: "force", short: "f", description: "Force removal of running VMs", takesArg: false }],
  },
  {
    name: "exec",
    description: "Execute a command inside a running VM",
    positionals: [{ completeWith: "vmId", variadic: false, description: "VM ID" }],
    flags: [
      { long: "sudo", description: "Run with sudo", takesArg: false },
      { long: "interactive", short: "i", description: "Interactive PTY mode", takesArg: false },
      {
        long: "no-extend-timeout",
        description: "Skip timeout extension (interactive only)",
        takesArg: false,
      },
      { long: "tty", short: "t", description: "Allocate a pseudo-TTY", takesArg: false },
      { long: "workdir", short: "w", description: "Working directory inside the VM", takesArg: true },
      { long: "env", short: "e", description: "Environment variable KEY=VAL", takesArg: true },
    ],
  },
  {
    name: "connect",
    description: "Connect to a running VM shell",
    positionals: [{ completeWith: "vmId", variadic: false, description: "VM ID" }],
    flags: [{ long: "session", short: "s", description: "Attach to existing session ID", takesArg: true }],
  },
  {
    name: "upload",
    description: "Upload local files to a running VM",
    positionals: [
      { completeWith: "vmId", variadic: false, description: "VM ID" },
      { completeWith: "file", variadic: true, description: "Local file" },
    ],
    flags: [{ long: "dest", short: "d", description: "Destination directory inside the VM", takesArg: true }],
  },
  {
    name: "download",
    description: "Download a file from a running VM",
    positionals: [
      { completeWith: "vmId", variadic: false, description: "VM ID" },
      { completeWith: "none", variadic: false, description: "Remote path" },
    ],
    flags: [
      {
        long: "dest",
        short: "d",
        description: "Local destination path",
        takesArg: true,
        completeWith: "file",
      },
    ],
  },
  {
    name: "network",
    description: "Update network policy on a running VM",
    positionals: [{ completeWith: "vmId", variadic: false, description: "VM ID" }],
    flags: [
      {
        long: "network-policy",
        description: "Network mode",
        takesArg: true,
        choices: ["allow-all", "deny-all", "custom"],
      },
      { long: "allowed-domain", description: "Domains to allow (comma-separated)", takesArg: true },
      { long: "allowed-cidr", description: "CIDRs to allow (comma-separated)", takesArg: true },
      { long: "denied-cidr", description: "CIDRs to deny (comma-separated)", takesArg: true },
    ],
  },
  {
    name: "doctor",
    description: "Check system prerequisites and installation health",
    positionals: [],
    flags: [],
  },
  {
    name: "completion",
    description: "Generate shell tab completion script",
    positionals: [{ completeWith: "shell", variadic: false, description: "Shell" }],
    flags: [],
  },
];
