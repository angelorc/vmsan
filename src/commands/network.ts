import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { vmsanPaths } from "../paths.ts";
import { VMService } from "../services/vm.ts";
import { createCommandLogger, getOutputMode } from "../lib/logger/index.ts";
import { handleCommandError } from "../errors/index.ts";
import {
  parseNetworkPolicy,
  parseDomains,
  parseCidrList,
  validateCidr,
} from "./create/validation.ts";
import { policyConflictError } from "../errors/index.ts";

const networkCommand = defineCommand({
  meta: {
    name: "network",
    description: "Update network policy on a running VM",
  },
  args: {
    vmId: {
      type: "positional",
      description: "VM to update",
      required: true,
    },
    "network-policy": {
      type: "string",
      required: true,
      description: "Base network mode: allow-all, deny-all, or custom",
    },
    "allowed-domain": {
      type: "string",
      description: "Domains/patterns to allow (comma-separated)",
    },
    "allowed-cidr": {
      type: "string",
      description: "Address ranges to allow (comma-separated CIDR)",
    },
    "denied-cidr": {
      type: "string",
      description: "Address ranges to deny (comma-separated CIDR)",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("network");

    try {
      const policy = parseNetworkPolicy(args["network-policy"]);
      const domains = parseDomains(args["allowed-domain"]);
      const allowedCidrs = parseCidrList(args["allowed-cidr"]);
      const deniedCidrs = parseCidrList(args["denied-cidr"]);

      for (const cidr of allowedCidrs) validateCidr(cidr);
      for (const cidr of deniedCidrs) validateCidr(cidr);

      if (policy === "deny-all" && (domains.length || allowedCidrs.length || deniedCidrs.length)) {
        throw policyConflictError();
      }

      const service = new VMService(vmsanPaths());
      const result = await service.updateNetworkPolicy(
        args.vmId,
        policy,
        domains,
        allowedCidrs,
        deniedCidrs,
      );

      if (!result.success) {
        if (result.error) throw result.error;
        consola.error(`Failed to update network policy for ${args.vmId}`);
        process.exitCode = 1;
        cmdLog.emit();
        return;
      }

      const outputMode = getOutputMode();
      if (outputMode === "json") {
        console.log(JSON.stringify(result));
      } else {
        consola.success(
          `Network policy updated for ${args.vmId}: ${result.previousPolicy} → ${result.newPolicy}`,
        );
        if (domains.length > 0) {
          consola.info(`Allowed domains: ${domains.join(", ")}`);
        }
        if (allowedCidrs.length > 0) {
          consola.info(`Allowed CIDRs: ${allowedCidrs.join(", ")}`);
        }
        if (deniedCidrs.length > 0) {
          consola.info(`Denied CIDRs: ${deniedCidrs.join(", ")}`);
        }
      }

      cmdLog.set({
        vmId: args.vmId,
        previousPolicy: result.previousPolicy,
        newPolicy: result.newPolicy,
        domains,
        allowedCidrs,
        deniedCidrs,
      });
      cmdLog.emit();
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default networkCommand as CommandDef;
