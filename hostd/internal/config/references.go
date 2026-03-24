package config

import (
	"fmt"
	"strings"
)

// ResolvedEnv maps environment variable names to their values for a service.
type ResolvedEnv map[string]string

// GetServiceVariables returns the auto-generated environment variables for a
// service based on its type and mesh IP. These are the variables available
// via ${{service.VAR}} references.
func GetServiceVariables(serviceName, serviceType, meshIP string) ResolvedEnv {
	vars := make(ResolvedEnv)
	upper := strings.ToUpper(strings.ReplaceAll(serviceName, "-", "_"))

	switch serviceType {
	case "postgres":
		vars["DATABASE_URL"] = fmt.Sprintf("postgres://vmsan:vmsan@%s:5432/app", meshIP)
		vars["PGHOST"] = meshIP
		vars["PGPORT"] = "5432"
		vars["PGUSER"] = "vmsan"
		vars["PGPASSWORD"] = "vmsan"
		vars["PGDATABASE"] = "app"
	case "redis":
		vars["REDIS_URL"] = fmt.Sprintf("redis://%s:6379", meshIP)
		vars["REDIS_HOST"] = meshIP
		vars["REDIS_PORT"] = "6379"
	case "mysql":
		vars["DATABASE_URL"] = fmt.Sprintf("mysql://vmsan:vmsan@%s:3306/app", meshIP)
		vars["MYSQL_HOST"] = meshIP
		vars["MYSQL_PORT"] = "3306"
		vars["MYSQL_USER"] = "vmsan"
		vars["MYSQL_PASSWORD"] = "vmsan"
		vars["MYSQL_DATABASE"] = "app"
	default:
		vars[upper+"_URL"] = fmt.Sprintf("http://%s:8080", meshIP)
		vars[upper+"_HOST"] = meshIP
	}

	return vars
}

// ResolveReferences replaces ${{service.VAR}} placeholders in env values
// with their resolved values from availableVars.
// availableVars maps "service.VAR" to the resolved value.
func ResolveReferences(env map[string]string, availableVars map[string]string) map[string]string {
	resolved := make(map[string]string, len(env))
	for k, v := range env {
		resolved[k] = resolveValue(v, availableVars)
	}
	return resolved
}

func resolveValue(value string, vars map[string]string) string {
	result := value
	for {
		start := strings.Index(result, "${{")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}}")
		if end == -1 {
			break
		}
		end += start + 2
		ref := strings.TrimSpace(result[start+3 : end-2])
		replacement, ok := vars[ref]
		if !ok {
			replacement = result[start:end] // keep unresolved
		}
		result = result[:start] + replacement + result[end:]
	}
	return result
}
