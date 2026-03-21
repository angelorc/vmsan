import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { consola } from "consola";
import { createVmsan } from "../context.ts";
import { createCommandLogger, getOutputMode } from "../lib/logger/index.ts";
import { handleCommandError, mutuallyExclusiveFlagsError } from "../errors/index.ts";
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
    "allow-icmp": {
      type: "boolean",
      description: "Allow ICMP traffic (ping) from the VM",
    },
    "deny-icmp": {
      type: "boolean",
      description: "Deny ICMP traffic (ping) from the VM",
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

      if (args["allow-icmp"] && args["deny-icmp"]) {
        throw mutuallyExclusiveFlagsError("--allow-icmp", "--deny-icmp");
      }

      // undefined = no change, true = allow, false = deny
      const allowIcmp = args["allow-icmp"] ? true : args["deny-icmp"] ? false : undefined;

      const vmsan = await createVmsan();
      const result = await vmsan.updateNetworkPolicy(
        args.vmId,
        policy,
        domains,
        allowedCidrs,
        deniedCidrs,
        allowIcmp,
      );

      if (!result.success) {
        if (result.error) throw result.error;
        consola.error(`Failed to update network policy for ${args.vmId}`);
        process.exitCode = 1;
        cmdLog.emit();
        return;
      }

      if (getOutputMode() !== "json") {
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
