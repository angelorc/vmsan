/**
 * Validation logic for vmsan.toml configuration files.
 *
 * Checks: syntax, missing dependencies, circular dependencies,
 * invalid ports, unknown runtimes, missing required fields,
 * and duplicate service names.
 */

import { parse, TomlError } from "smol-toml";
import type { VmsanToml, ServiceConfig } from "./types.ts";
import { findClosestMatch as findClosest } from "./suggest.ts";
import { VALID_RUNTIMES } from "../../commands/create/types.ts";

export interface ValidationError {
  line?: number;
  field: string;
  message: string;
  suggestion?: string;
}

/**
 * Parse raw TOML text and return the config or syntax errors.
 */
export function parseTomlSafe(tomlText: string): {
  config: VmsanToml | null;
  errors: ValidationError[];
} {
  try {
    const config = parse(tomlText) as unknown as VmsanToml;
    return { config, errors: [] };
  } catch (error) {
    if (error instanceof TomlError) {
      return {
        config: null,
        errors: [
          {
            line: error.line,
            field: "syntax",
            message: error.message,
          },
        ],
      };
    }
    return {
      config: null,
      errors: [
        {
          field: "syntax",
          message: error instanceof Error ? error.message : String(error),
        },
      ],
    };
  }
}

/**
 * Validate a parsed VmsanToml config. Returns an array of validation errors.
 * An empty array means the config is valid.
 */
export function validateToml(config: VmsanToml): ValidationError[] {
  const errors: ValidationError[] = [];

  // Collect all service + accessory names
  const allNames = new Set<string>();
  const serviceNames = new Set(Object.keys(config.services ?? {}));
  const accessoryNames = new Set(Object.keys(config.accessories ?? {}));

  for (const name of serviceNames) {
    allNames.add(name);
  }
  for (const name of accessoryNames) {
    allNames.add(name);
  }

  // Check for duplicate names across services and accessories
  for (const name of serviceNames) {
    if (accessoryNames.has(name)) {
      errors.push({
        field: `services.${name}`,
        message: `Duplicate name "${name}" — used in both services and accessories`,
      });
    }
  }

  // Validate services
  if (config.services) {
    for (const [name, service] of Object.entries(config.services)) {
      validateService(name, service, allNames, errors);
    }

    // Check circular dependencies
    const circularErrors = detectCircularDependencies(config.services);
    errors.push(...circularErrors);
  }

  return errors;
}

function validateService(
  name: string,
  service: ServiceConfig,
  allNames: Set<string>,
  errors: ValidationError[],
): void {
  // Missing start command
  if (!service.start) {
    errors.push({
      field: `services.${name}.start`,
      message: `Service "${name}" is missing a "start" command`,
    });
  }

  // Unknown runtime
  if (
    service.runtime &&
    !VALID_RUNTIMES.includes(service.runtime as (typeof VALID_RUNTIMES)[number])
  ) {
    const suggestion = findClosestMatch(service.runtime, [...VALID_RUNTIMES]);
    errors.push({
      field: `services.${name}.runtime`,
      message: `Unknown runtime "${service.runtime}" in service "${name}"`,
      suggestion: suggestion
        ? `Did you mean "${suggestion}"?`
        : `Valid runtimes: ${VALID_RUNTIMES.join(", ")}`,
    });
  }

  // Missing dependencies
  if (service.depends_on) {
    for (const dep of service.depends_on) {
      if (!allNames.has(dep)) {
        const suggestion = findClosestMatch(dep, [...allNames]);
        errors.push({
          field: `services.${name}.depends_on`,
          message: `Service "${name}" references unknown service "${dep}" in depends_on`,
          suggestion: suggestion ? `Did you mean "${suggestion}"?` : undefined,
        });
      }
    }
  }

  // Invalid ports in connect_to
  if (service.connect_to) {
    for (const entry of service.connect_to) {
      const colonIndex = entry.lastIndexOf(":");
      if (colonIndex === -1) {
        errors.push({
          field: `services.${name}.connect_to`,
          message: `Invalid connect_to entry "${entry}" in service "${name}": expected format "service:port"`,
        });
        continue;
      }

      const targetService = entry.slice(0, colonIndex);
      const portStr = entry.slice(colonIndex + 1);
      const port = Number(portStr);

      if (!allNames.has(targetService)) {
        const suggestion = findClosestMatch(targetService, [...allNames]);
        errors.push({
          field: `services.${name}.connect_to`,
          message: `Service "${name}" references unknown service "${targetService}" in connect_to`,
          suggestion: suggestion ? `Did you mean "${suggestion}"?` : undefined,
        });
      }

      if (Number.isNaN(port) || port < 1 || port > 65535 || !Number.isInteger(port)) {
        errors.push({
          field: `services.${name}.connect_to`,
          message: `Invalid port "${portStr}" in connect_to entry "${entry}" for service "${name}"`,
          suggestion: "Port must be an integer between 1 and 65535",
        });
      }
    }
  }
}

/**
 * Detect circular dependencies in the service dependency graph.
 */
function detectCircularDependencies(services: Record<string, ServiceConfig>): ValidationError[] {
  const errors: ValidationError[] = [];
  const visited = new Set<string>();
  const inStack = new Set<string>();

  function dfs(name: string, path: string[]): boolean {
    if (inStack.has(name)) {
      const cycleStart = path.indexOf(name);
      const cycle = path.slice(cycleStart).concat(name);
      errors.push({
        field: "services",
        message: `Circular dependency detected: ${cycle.join(" -> ")}`,
      });
      return true;
    }

    if (visited.has(name)) return false;

    visited.add(name);
    inStack.add(name);

    const service = services[name];
    if (service?.depends_on) {
      for (const dep of service.depends_on) {
        if (services[dep]) {
          dfs(dep, [...path, name]);
        }
      }
    }

    inStack.delete(name);
    return false;
  }

  for (const name of Object.keys(services)) {
    if (!visited.has(name)) {
      dfs(name, []);
    }
  }

  return errors;
}

function findClosestMatch(input: string, candidates: string[]): string | null {
  return findClosest(input, candidates);
}
