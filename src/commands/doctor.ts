import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createCommandLogger, getOutputMode } from "../lib/logger/index.ts";
import { handleCommandError } from "../errors/index.ts";
import { GatewayClient, type GatewayDoctorCheck } from "../lib/gateway-client.ts";

interface CheckResult {
  category: string;
  name: string;
  status: "pass" | "fail" | "warn";
  detail: string;
  fix?: string;
}

function mapGatewayCheck(check: GatewayDoctorCheck): CheckResult {
  return {
    category: check.category,
    name: check.name,
    status: check.status,
    detail: check.detail,
    fix: check.fix,
  };
}

export async function runDoctorChecks(): Promise<CheckResult[]> {
  const gateway = new GatewayClient();
  const result = await gateway.doctor();
  if (!result.ok) {
    throw new Error(result.error ?? "Gateway doctor RPC failed");
  }
  return (result.list ?? []).map(mapGatewayCheck);
}

const PASS = "\x1b[32mok\x1b[0m";
const FAIL = "\x1b[31mFAIL\x1b[0m";
const WARN = "\x1b[33mWARN\x1b[0m";

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
    const statusStr = check.status === "pass" ? PASS : check.status === "warn" ? WARN : FAIL;
    const detail = check.detail;
    lines.push(`    ${check.name} ${dots} ${statusStr} (${detail})`);

    if (check.status !== "pass" && check.fix) {
      lines.push(`      \x1b[33mFix: ${check.fix}\x1b[0m`);
    }
  }

  const passed = checks.filter((c) => c.status === "pass").length;
  const failed = checks.filter((c) => c.status === "fail").length;
  const warned = checks.filter((c) => c.status === "warn").length;
  lines.push("");
  lines.push(`  Result: ${passed} passed, ${failed} failed${warned > 0 ? `, ${warned} warnings` : ""}`);

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
      const checks = await runDoctorChecks();
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
