/**
 * Reference variable resolution for vmsan.toml.
 *
 * Pattern: ${{service.VAR}}
 * Auto-generates environment variables per service type (postgres, redis, etc.)
 * and resolves cross-service references.
 */

const REF_PATTERN = /\$\{\{([^}]+)\}\}/g;

export interface ResolvedEnv {
  [key: string]: string;
}

/**
 * Auto-generated variables per service type.
 * These become available as `${{serviceName.VAR}}` in other services' env blocks.
 */
export function getServiceVariables(
  serviceName: string,
  serviceType: string,
  meshIp: string,
): ResolvedEnv {
  switch (serviceType) {
    case "postgres":
      return {
        DATABASE_URL: `postgres://vmsan:vmsan@${meshIp}:5432/${serviceName}`,
        PGHOST: meshIp,
        PGPORT: "5432",
        PGUSER: "vmsan",
        PGPASSWORD: "vmsan",
        PGDATABASE: serviceName,
      };
    case "redis":
      return {
        REDIS_URL: `redis://${meshIp}:6379`,
        REDIS_HOST: meshIp,
        REDIS_PORT: "6379",
      };
    case "mysql":
      return {
        DATABASE_URL: `mysql://vmsan:vmsan@${meshIp}:3306/${serviceName}`,
        MYSQL_HOST: meshIp,
        MYSQL_PORT: "3306",
        MYSQL_USER: "vmsan",
        MYSQL_PASSWORD: "vmsan",
        MYSQL_DATABASE: serviceName,
      };
    default:
      return {
        [`${serviceName.toUpperCase()}_URL`]: `http://${meshIp}:8080`,
        [`${serviceName.toUpperCase()}_HOST`]: meshIp,
      };
  }
}

/**
 * Resolve `${{service.VAR}}` references in an env map.
 *
 * @param env - The environment variables to resolve (may contain `${{...}}` patterns)
 * @param availableVars - Map of service name to its resolved variables
 * @returns Resolved environment variables
 * @throws Error when a reference points to an unknown service or variable
 */
export function resolveReferences(
  env: Record<string, string>,
  availableVars: Record<string, ResolvedEnv>,
): Record<string, string> {
  const resolved: Record<string, string> = {};

  for (const [key, value] of Object.entries(env)) {
    resolved[key] = value.replace(REF_PATTERN, (_match, ref: string) => {
      const dotIndex = ref.indexOf(".");
      if (dotIndex === -1) {
        throw new Error(`Invalid reference "$\{{${ref}}}": expected format "service.VAR"`);
      }

      const serviceName = ref.slice(0, dotIndex);
      const varName = ref.slice(dotIndex + 1);

      const serviceVars = availableVars[serviceName];
      if (!serviceVars) {
        const available = Object.keys(availableVars).join(", ");
        throw new Error(
          `Unknown service "${serviceName}" in reference "$\{{${ref}}}". Available: ${available}`,
        );
      }

      const resolvedValue = serviceVars[varName];
      if (resolvedValue === undefined) {
        const available = Object.keys(serviceVars).join(", ");
        throw new Error(
          `Unknown variable "${varName}" for service "${serviceName}" in reference "$\{{${ref}}}". Available: ${available}`,
        );
      }

      return resolvedValue;
    });
  }

  return resolved;
}
