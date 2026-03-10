import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { accessSync, constants, existsSync, readFileSync, readdirSync, statfsSync } from "node:fs";
import { execSync } from "node:child_process";
import { basename, join } from "node:path";
import { consola } from "consola";
import { createCommandLogger, getOutputMode } from "../lib/logger/index.ts";
import { handleCommandError } from "../errors/index.ts";
import type { VmsanPaths } from "../paths.ts";
import { vmsanPaths } from "../paths.ts";

interface CheckResult {
  category: string;
  name: string;
  status: "pass" | "fail";
  detail: string;
  fix?: string;
}

function checkKvm(): CheckResult {
  try {
    accessSync("/dev/kvm", constants.R_OK | constants.W_OK);
    return { category: "System", name: "KVM", status: "pass", detail: "/dev/kvm" };
  } catch {
    return {
      category: "System",
      name: "KVM",
      status: "fail",
      detail: "KVM not available",
      fix: "Enable KVM in BIOS or load the kvm module: sudo modprobe kvm",
    };
  }
}

function checkDiskSpace(baseDir: string): CheckResult {
  try {
    const stats = statfsSync(baseDir);
    const freeBytes = stats.bfree * stats.bsize;
    const freeGB = Math.floor(freeBytes / 1_073_741_824);
    if (freeGB >= 5) {
      return {
        category: "System",
        name: "Disk space",
        status: "pass",
        detail: `${freeGB} GB free`,
      };
    }
    return {
      category: "System",
      name: "Disk space",
      status: "fail",
      detail: `Low disk space (${freeGB} GB free)`,
      fix: "Free up disk space. vmsan needs at least 5 GB.",
    };
  } catch {
    return {
      category: "System",
      name: "Disk space",
      status: "fail",
      detail: "Could not check disk space",
      fix: `Ensure ${baseDir} exists and is accessible.`,
    };
  }
}

function checkDefaultInterface(): CheckResult {
  try {
    const output = execSync("ip route show default", { encoding: "utf-8", stdio: "pipe" }).trim();
    const match = output.match(/dev\s+(\S+)/);
    if (match) {
      return { category: "System", name: "Default interface", status: "pass", detail: match[1] };
    }
    return {
      category: "System",
      name: "Default interface",
      status: "fail",
      detail: "No default route",
      fix: "Configure a default network route.",
    };
  } catch {
    return {
      category: "System",
      name: "Default interface",
      status: "fail",
      detail: "No default route",
      fix: "Configure a default network route.",
    };
  }
}

function checkTunDevice(): CheckResult {
  try {
    accessSync("/dev/net/tun", constants.R_OK | constants.W_OK);
    return { category: "System", name: "TUN device", status: "pass", detail: "/dev/net/tun" };
  } catch {
    return {
      category: "System",
      name: "TUN device",
      status: "fail",
      detail: "/dev/net/tun not accessible",
      fix: "Load the tun kernel module: sudo modprobe tun",
    };
  }
}

function checkJailerFilesystem(jailerBaseDir: string): CheckResult {
  try {
    const mounts = readFileSync("/proc/mounts", "utf-8");
    let bestMatch = "";
    let bestOptions = "";
    for (const line of mounts.split("\n")) {
      const parts = line.split(" ");
      if (parts.length < 4) continue;
      const mountpoint = parts[1];
      const isAncestor =
        jailerBaseDir === mountpoint ||
        jailerBaseDir.startsWith(mountpoint.endsWith("/") ? mountpoint : `${mountpoint}/`);
      if (isAncestor && mountpoint.length > bestMatch.length) {
        bestMatch = mountpoint;
        bestOptions = parts[3];
      }
    }
    if (!bestMatch) {
      return {
        category: "System",
        name: "Jailer filesystem",
        status: "pass",
        detail: "Check skipped",
      };
    }
    const options = bestOptions.split(",");
    if (options.includes("nodev")) {
      return {
        category: "System",
        name: "Jailer filesystem",
        status: "fail",
        detail: `${bestMatch} mounted with nodev`,
        fix: `The jailer needs device nodes to work. Remount without nodev: sudo mount -o remount,dev ${bestMatch}`,
      };
    }
    return { category: "System", name: "Jailer filesystem", status: "pass", detail: bestMatch };
  } catch {
    return {
      category: "System",
      name: "Jailer filesystem",
      status: "pass",
      detail: "Check skipped",
    };
  }
}

function checkFirecracker(binDir: string): CheckResult {
  const fcPath = join(binDir, "firecracker");
  if (!existsSync(fcPath)) {
    return {
      category: "Binaries",
      name: "Firecracker",
      status: "fail",
      detail: "Not found",
      fix: 'Run "curl -fsSL https://vmsan.dev/install | bash" to install.',
    };
  }
  try {
    const version = execSync(`"${fcPath}" --version`, { encoding: "utf-8", stdio: "pipe" }).trim();
    return { category: "Binaries", name: "Firecracker", status: "pass", detail: version };
  } catch {
    return { category: "Binaries", name: "Firecracker", status: "pass", detail: "Found" };
  }
}

function checkJailer(binDir: string): CheckResult {
  const jailerPath = join(binDir, "jailer");
  if (!existsSync(jailerPath)) {
    return {
      category: "Binaries",
      name: "Jailer",
      status: "fail",
      detail: "Not found",
      fix: 'Run "curl -fsSL https://vmsan.dev/install | bash" to install.',
    };
  }
  return { category: "Binaries", name: "Jailer", status: "pass", detail: "Found" };
}

function checkAgent(agentBin: string): CheckResult {
  if (!existsSync(agentBin)) {
    return {
      category: "Binaries",
      name: "Agent",
      status: "fail",
      detail: "Not found",
      fix: 'Run "curl -fsSL https://vmsan.dev/install | bash" to install.',
    };
  }
  return { category: "Binaries", name: "Agent", status: "pass", detail: "Found" };
}

function checkNftablesBinary(nftablesBin: string): CheckResult {
  if (!existsSync(nftablesBin)) {
    return {
      category: "Binaries",
      name: "vmsan-nftables",
      status: "fail",
      detail: "Not found",
      fix: 'Run "curl -fsSL https://vmsan.dev/install | bash" to install.',
    };
  }
  return { category: "Binaries", name: "vmsan-nftables", status: "pass", detail: "Found" };
}

function checkNftablesKernel(nftablesBin: string): CheckResult {
  try {
    // Use vmsan-nftables verify (netlink-based) instead of the nft CLI.
    // vmsan-nftables uses netlink directly and doesn't require the nft package.
    execSync(`"${nftablesBin}" verify`, {
      input: '{"vmId":"_doctor_probe"}',
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    });
    return {
      category: "System",
      name: "nftables kernel",
      status: "pass",
      detail: "nftables kernel support verified",
    };
  } catch {
    return {
      category: "System",
      name: "nftables kernel",
      status: "fail",
      detail: "nftables kernel support not available",
      fix: "Load the nftables kernel module: sudo modprobe nf_tables",
    };
  }
}

function checkHostFirewall(): CheckResult {
  const active: string[] = [];
  try {
    const ufwOutput = execSync("ufw status", { encoding: "utf-8", stdio: "pipe" }).trim();
    if (ufwOutput.includes("Status: active")) {
      active.push("ufw");
    }
  } catch {
    // ufw not installed or not accessible
  }
  try {
    execSync("systemctl is-active firewalld", { encoding: "utf-8", stdio: "pipe" });
    active.push("firewalld");
  } catch {
    // firewalld not active or not installed
  }

  if (active.length > 0) {
    return {
      category: "System",
      name: "Host firewall",
      status: "fail",
      detail: `${active.join(", ")} active`,
      fix: `Disable ${active.join(" and ")} or add rules to allow vmsan traffic. Host firewalls can conflict with vmsan's nftables rules.`,
    };
  }
  return { category: "System", name: "Host firewall", status: "pass", detail: "No conflicts" };
}

function checkKernel(kernelsDir: string): CheckResult {
  try {
    if (!existsSync(kernelsDir)) {
      return {
        category: "Images",
        name: "Kernel",
        status: "fail",
        detail: "Not found",
        fix: 'Run "curl -fsSL https://vmsan.dev/install | bash" to install.',
      };
    }
    const files = readdirSync(kernelsDir).filter((f) => f.startsWith("vmlinux"));
    if (files.length === 0) {
      return {
        category: "Images",
        name: "Kernel",
        status: "fail",
        detail: "Not found",
        fix: 'Run "curl -fsSL https://vmsan.dev/install | bash" to install.',
      };
    }
    const latest = files.sort().at(-1)!;
    return { category: "Images", name: "Kernel", status: "pass", detail: latest };
  } catch {
    return {
      category: "Images",
      name: "Kernel",
      status: "fail",
      detail: "Not found",
      fix: 'Run "curl -fsSL https://vmsan.dev/install | bash" to install.',
    };
  }
}

function checkRootfs(rootfsDir: string): CheckResult {
  const rootfsPath = join(rootfsDir, "ubuntu-24.04.ext4");
  if (existsSync(rootfsPath)) {
    return {
      category: "Images",
      name: "Rootfs (base)",
      status: "pass",
      detail: basename(rootfsPath),
    };
  }
  return {
    category: "Images",
    name: "Rootfs (base)",
    status: "fail",
    detail: "Not found",
    fix: 'Run "curl -fsSL https://vmsan.dev/install | bash" to install.',
  };
}

export function runDoctorChecks(paths?: VmsanPaths): CheckResult[] {
  const p = paths ?? vmsanPaths();
  return [
    checkKvm(),
    checkTunDevice(),
    checkDiskSpace(p.baseDir),
    checkDefaultInterface(),
    checkNftablesKernel(p.nftablesBin),
    checkHostFirewall(),
    checkJailerFilesystem(p.jailerBaseDir),
    checkFirecracker(p.binDir),
    checkJailer(p.binDir),
    checkAgent(p.agentBin),
    checkNftablesBinary(p.nftablesBin),
    checkKernel(p.kernelsDir),
    checkRootfs(p.rootfsDir),
  ];
}

const PASS = "\x1b[32mok\x1b[0m";
const FAIL = "\x1b[31mFAIL\x1b[0m";

function formatHumanOutput(checks: CheckResult[]): string {
  const lines: string[] = [];
  let currentCategory = "";

  for (const check of checks) {
    if (check.category !== currentCategory) {
      if (currentCategory) lines.push("");
      lines.push(`  ${check.category}`);
      currentCategory = check.category;
    }

    const dots = ".".repeat(Math.max(1, 30 - check.name.length));
    const statusStr = check.status === "pass" ? PASS : FAIL;
    const detail = check.detail;
    lines.push(`    ${check.name} ${dots} ${statusStr} (${detail})`);

    if (check.status === "fail" && check.fix) {
      lines.push(`      \x1b[33mFix: ${check.fix}\x1b[0m`);
    }
  }

  const passed = checks.filter((c) => c.status === "pass").length;
  const failed = checks.filter((c) => c.status === "fail").length;
  lines.push("");
  lines.push(`  Result: ${passed} passed, ${failed} failed`);

  return lines.join("\n");
}

const doctorCommand = defineCommand({
  meta: {
    name: "doctor",
    description: "Check system prerequisites and vmsan installation health",
  },
  async run() {
    const cmdLog = createCommandLogger("doctor");

    try {
      const checks = runDoctorChecks();
      const passed = checks.filter((c) => c.status === "pass").length;
      const failed = checks.filter((c) => c.status === "fail").length;

      if (getOutputMode() === "json") {
        cmdLog.set({
          checks: checks.map(({ fix: _fix, ...rest }) => rest),
          summary: { passed, failed, total: checks.length },
        });
      } else {
        consola.log("");
        consola.log("vmsan doctor\n");
        consola.log(formatHumanOutput(checks));
        cmdLog.set({ passed, failed, total: checks.length });
      }

      cmdLog.emit();

      if (failed > 0) {
        process.exitCode = 1;
      }
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default doctorCommand as CommandDef;
