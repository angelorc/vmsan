import consola from "consola";
import type { VMService } from "../../services/vm.ts";
import type { VmsanToml, ServiceConfig, AccessoryConfig } from "../toml/parser.ts";
import { normalizeToml } from "../toml/parser.ts";
import { buildDependencyGraph } from "./graph.ts";
import { deployService, type DeployStatus, type ServiceDeployResult } from "./engine.ts";
import { resolveReferences, getServiceVariables, type ResolvedEnv } from "../toml/references.ts";
import { toError } from "../utils.ts";

// ── Types ───────────────────────────────────────────────────────────────────

export interface OrchestrateOptions {
  /** Parsed vmsan.toml */
  config: VmsanToml;
  /** Project name */
  project: string;
  /** Source directory */
  sourceDir: string;
  /** VMService instance */
  vmService: VMService;
  /** Additional env vars */
  env?: Record<string, string>;
}

export interface OrchestrateResult {
  services: ServiceDeployResult[];
  success: boolean;
  durationMs: number;
}

// ── Main orchestrator ───────────────────────────────────────────────────────

export async function orchestrateDeploy(opts: OrchestrateOptions): Promise<OrchestrateResult> {
  const { config, project, sourceDir, vmService, env } = opts;
  const startTime = Date.now();
  const allResults: ServiceDeployResult[] = [];

  try {
    // 1. Normalize services (handles single-service → multi-service conversion)
    const services = normalizeToml(config);
    const accessories = config.accessories ?? {};

    // 2. Build unified dependency-aware map
    //    Services have depends_on; accessories are leaf nodes (no deps)
    const depInput: Record<string, { depends_on?: string[] }> = {};
    for (const [name, svc] of Object.entries(services)) {
      depInput[name] = { depends_on: svc.depends_on };
    }

    // 3. Build dependency graph
    const graph = buildDependencyGraph(depInput, accessories);

    if (graph.groups.length === 0) {
      consola.warn("No services to deploy");
      return { services: [], success: true, durationMs: Date.now() - startTime };
    }

    consola.info(`Deploying ${graph.order.length} service(s) in ${graph.groups.length} group(s)`);
    consola.debug(`Deploy order: ${graph.order.join(" -> ")}`);

    // 4. Build a map for reference variable resolution
    //    After each group deploys, we'll populate vars from the deployed VMs
    const resolvedVars: Record<string, ResolvedEnv> = {};

    // 5. Deploy groups in topological order
    for (const group of graph.groups) {
      consola.info(`Deploying group ${group.level}: ${group.services.join(", ")}`);

      // Deploy all services in the group in parallel
      const groupPromises = group.services.map(async (serviceName) => {
        const isAccessory = serviceName in accessories;
        const accessoryConfig = isAccessory ? accessories[serviceName] : undefined;

        // Build service config: for accessories, create a minimal ServiceConfig
        let serviceConfig: ServiceConfig;
        if (isAccessory) {
          serviceConfig = {
            memory: 256,
            vcpus: 1,
            ...services[serviceName], // In case it's somehow in both
          };
        } else {
          serviceConfig = services[serviceName];
          if (!serviceConfig) {
            return {
              service: serviceName,
              status: "failed" as DeployStatus,
              vmId: null,
              error: `Service "${serviceName}" not found in config`,
              durationMs: 0,
            } satisfies ServiceDeployResult;
          }
        }

        // Resolve reference variables in env
        let resolvedEnv: Record<string, string> = { ...serviceConfig.env, ...env };
        if (serviceConfig.env && Object.keys(resolvedVars).length > 0) {
          try {
            resolvedEnv = {
              ...resolveReferences(serviceConfig.env, resolvedVars),
              ...env,
            };
          } catch (err) {
            consola.debug(`[${serviceName}] Reference resolution: ${toError(err).message}`);
          }
        }

        return deployService({
          name: serviceName,
          config: serviceConfig,
          deployConfig: config.deploy,
          project,
          sourceDir,
          vmService,
          env: resolvedEnv,
          accessory: accessoryConfig,
        });
      });

      const settled = await Promise.allSettled(groupPromises);
      let groupFailed = false;

      for (const result of settled) {
        if (result.status === "fulfilled") {
          allResults.push(result.value);
          if (result.value.status === "failed") {
            groupFailed = true;
          }

          // After successful deploy, populate reference variables
          if (result.value.vmId && result.value.status === "running") {
            const vm = vmService.get(result.value.vmId);
            if (vm) {
              const serviceName = result.value.service;
              const accessoryType = accessories[serviceName]?.type;
              const meshIp = vm.network.meshIp ?? vm.network.guestIp;
              resolvedVars[serviceName] = getServiceVariables(
                serviceName,
                accessoryType ?? "service",
                meshIp,
              );
            }
          }
        } else {
          // Promise rejected (unexpected)
          const errorMessage = toError(result.reason).message;
          consola.error(`Unexpected error in group ${group.level}: ${errorMessage}`);
          allResults.push({
            service: "unknown",
            status: "failed",
            vmId: null,
            error: errorMessage,
            durationMs: 0,
          });
          groupFailed = true;
        }
      }

      if (groupFailed) {
        consola.error(`Group ${group.level} had failures, aborting remaining groups`);
        break;
      }
    }

    const success = allResults.every((r) => r.status === "running" || r.status === "skipped");
    return {
      services: allResults,
      success,
      durationMs: Date.now() - startTime,
    };
  } catch (err) {
    const errorMessage = toError(err).message;
    consola.error(`Orchestration failed: ${errorMessage}`);
    return {
      services: allResults,
      success: false,
      durationMs: Date.now() - startTime,
    };
  }
}
