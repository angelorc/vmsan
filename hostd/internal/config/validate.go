package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ValidationError represents a single validation issue.
type ValidationError struct {
	Line       int    `json:"line,omitempty"`
	Field      string `json:"field"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// ParseTomlSafe parses TOML and returns both the config and any syntax errors.
func ParseTomlSafe(tomlText string) (*VmsanToml, []ValidationError) {
	cfg, err := ParseVmsanToml(tomlText)
	if err != nil {
		return nil, []ValidationError{{
			Field:   "syntax",
			Message: err.Error(),
		}}
	}
	return cfg, nil
}

// ValidateToml validates a parsed VmsanToml config and returns all errors found.
func ValidateToml(cfg *VmsanToml) []ValidationError {
	var errs []ValidationError
	services := NormalizeToml(cfg)

	for name, svc := range services {
		errs = append(errs, validateService(name, svc, services, cfg.Accessories)...)
	}
	for name, acc := range cfg.Accessories {
		errs = append(errs, validateAccessory(name, acc)...)
	}

	// Check for circular dependencies
	if cycles := detectCycles(services, cfg.Accessories); len(cycles) > 0 {
		for _, cycle := range cycles {
			errs = append(errs, ValidationError{
				Field:   "depends_on",
				Message: fmt.Sprintf("Circular dependency detected: %s", strings.Join(cycle, " -> ")),
			})
		}
	}

	// Check for duplicate names across services and accessories
	for name := range services {
		if _, ok := cfg.Accessories[name]; ok {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("services.%s", name),
				Message: fmt.Sprintf("Duplicate name %q used in both services and accessories", name),
			})
		}
	}

	return errs
}

func validateService(name string, svc ServiceConfig, allServices map[string]ServiceConfig, accessories map[string]AccessoryConfig) []ValidationError {
	var errs []ValidationError

	// Missing start command
	if svc.Start == "" {
		errs = append(errs, ValidationError{
			Field:      fmt.Sprintf("services.%s.start", name),
			Message:    fmt.Sprintf("Service %q is missing a start command", name),
			Suggestion: "Add start = \"your-start-command\" to the service",
		})
	}

	// Unknown runtime
	if svc.Runtime != "" && !isValidRuntime(svc.Runtime) {
		suggestion := FindClosestMatch(svc.Runtime, ValidRuntimes)
		sugMsg := fmt.Sprintf("Valid runtimes: %s", strings.Join(ValidRuntimes, ", "))
		if suggestion != "" {
			sugMsg = fmt.Sprintf("Did you mean %q?", suggestion)
		}
		errs = append(errs, ValidationError{
			Field:      fmt.Sprintf("services.%s.runtime", name),
			Message:    fmt.Sprintf("Unknown runtime %q in service %q", svc.Runtime, name),
			Suggestion: sugMsg,
		})
	}

	// Missing dependency references
	for _, dep := range svc.DependsOn {
		if _, ok := allServices[dep]; !ok {
			if _, ok := accessories[dep]; !ok {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("services.%s.depends_on", name),
					Message: fmt.Sprintf("Service %q depends on %q which is not defined", name, dep),
				})
			}
		}
	}

	// Invalid ports in connect_to
	for _, ct := range svc.ConnectTo {
		parts := strings.SplitN(ct, ":", 2)
		if len(parts) == 2 {
			port, err := strconv.Atoi(parts[1])
			if err != nil || port < 1 || port > 65535 {
				errs = append(errs, ValidationError{
					Field:      fmt.Sprintf("services.%s.connect_to", name),
					Message:    fmt.Sprintf("Invalid port in connect_to %q for service %q", ct, name),
					Suggestion: "Ports must be between 1 and 65535",
				})
			}
		}
	}

	// Invalid publish_ports
	for _, port := range svc.PublishPorts {
		if port < 1 || port > 65535 {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("services.%s.publish_ports", name),
				Message: fmt.Sprintf("Invalid port %d in service %q", port, name),
			})
		}
	}

	// Validate network_policy
	if svc.NetworkPolicy != "" && !isValidNetworkPolicy(svc.NetworkPolicy) {
		errs = append(errs, ValidationError{
			Field:      fmt.Sprintf("services.%s.network_policy", name),
			Message:    fmt.Sprintf("Unknown network_policy %q in service %q", svc.NetworkPolicy, name),
			Suggestion: fmt.Sprintf("Valid policies: %s", strings.Join(ValidNetworkPolicies, ", ")),
		})
	}

	// Validate allowed_domains requires custom policy
	if len(svc.AllowedDomains) > 0 && svc.NetworkPolicy != "" && svc.NetworkPolicy != "custom" {
		errs = append(errs, ValidationError{
			Field:      fmt.Sprintf("services.%s.allowed_domains", name),
			Message:    fmt.Sprintf("Service %q has allowed_domains but network_policy is %q", name, svc.NetworkPolicy),
			Suggestion: "Set network_policy = \"custom\" to use allowed_domains",
		})
	}

	// Validate CIDR-like values in allowed_domains
	for _, domain := range svc.AllowedDomains {
		if _, _, err := net.ParseCIDR(domain); err == nil {
			errs = append(errs, ValidationError{
				Field:      fmt.Sprintf("services.%s.allowed_domains", name),
				Message:    fmt.Sprintf("Value %q looks like a CIDR, not a domain", domain),
				Suggestion: "Use allowed_cidrs for IP ranges",
			})
		}
	}

	return errs
}

func validateAccessory(name string, acc AccessoryConfig) []ValidationError {
	var errs []ValidationError

	if acc.Type == "" {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("accessories.%s.type", name),
			Message: fmt.Sprintf("Accessory %q is missing required field 'type'", name),
		})
	} else if !isValidAccessoryType(acc.Type) {
		errs = append(errs, ValidationError{
			Field:      fmt.Sprintf("accessories.%s.type", name),
			Message:    fmt.Sprintf("Unknown accessory type %q", acc.Type),
			Suggestion: fmt.Sprintf("Valid types: %s", strings.Join(ValidAccessoryTypes, ", ")),
		})
	}

	return errs
}

// detectCycles uses DFS three-color marking to find circular dependencies.
func detectCycles(services map[string]ServiceConfig, accessories map[string]AccessoryConfig) [][]string {
	const (
		white = 0 // unvisited
		gray  = 1 // visiting
		black = 2 // done
	)

	color := make(map[string]int)
	parent := make(map[string]string)
	var cycles [][]string

	// Collect all node names
	for name := range services {
		color[name] = white
	}
	for name := range accessories {
		color[name] = white
	}

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = gray

		var deps []string
		if svc, ok := services[node]; ok {
			deps = svc.DependsOn
		}

		for _, dep := range deps {
			if _, exists := color[dep]; !exists {
				continue // unknown dep, caught by other validation
			}
			switch color[dep] {
			case white:
				parent[dep] = node
				dfs(dep)
			case gray:
				// Build cycle path
				cycle := []string{dep}
				cur := node
				for cur != dep {
					cycle = append([]string{cur}, cycle...)
					cur = parent[cur]
				}
				cycle = append([]string{dep}, cycle...)
				cycles = append(cycles, cycle)
			}
		}

		color[node] = black
	}

	for name := range color {
		if color[name] == white {
			dfs(name)
		}
	}

	return cycles
}

func isValidRuntime(r string) bool {
	for _, v := range ValidRuntimes {
		if v == r {
			return true
		}
	}
	return false
}

func isValidNetworkPolicy(p string) bool {
	for _, v := range ValidNetworkPolicies {
		if v == p {
			return true
		}
	}
	return false
}

func isValidAccessoryType(t string) bool {
	for _, v := range ValidAccessoryTypes {
		if v == t {
			return true
		}
	}
	return false
}
