package config

// VmsanToml is the top-level configuration parsed from vmsan.toml.
type VmsanToml struct {
	// Flat single-service fields (optional, used when no [services] block)
	Project string `toml:"project,omitempty"`
	Runtime string `toml:"runtime,omitempty"`
	Build   string `toml:"build,omitempty"`
	Start   string `toml:"start,omitempty"`

	// Multi-service blocks
	Services    map[string]ServiceConfig   `toml:"services,omitempty"`
	Accessories map[string]AccessoryConfig `toml:"accessories,omitempty"`
	Deploy      DeployConfig               `toml:"deploy,omitempty"`
	Tunnel      TunnelConfig               `toml:"tunnel,omitempty"`
}

// ServiceConfig defines a single service (VM) within the project.
type ServiceConfig struct {
	Runtime        string            `toml:"runtime,omitempty"`
	Build          string            `toml:"build,omitempty"`
	Start          string            `toml:"start,omitempty"`
	Env            map[string]string `toml:"env,omitempty"`
	DependsOn      []string          `toml:"depends_on,omitempty"`
	ConnectTo      []string          `toml:"connect_to,omitempty"`
	Service        string            `toml:"service,omitempty"`
	PublishPorts   []int             `toml:"publish_ports,omitempty"`
	Memory         int               `toml:"memory,omitempty"`
	Vcpus          int               `toml:"vcpus,omitempty"`
	Disk           string            `toml:"disk,omitempty"`
	NetworkPolicy  string            `toml:"network_policy,omitempty"`
	AllowedDomains []string          `toml:"allowed_domains,omitempty"`
	HealthCheck    *HealthCheckConfig `toml:"health_check,omitempty"`
}

// AccessoryConfig defines an accessory service (database, cache, etc.).
type AccessoryConfig struct {
	Type    string            `toml:"type"`
	Version string            `toml:"version,omitempty"`
	Env     map[string]string `toml:"env,omitempty"`
}

// DeployConfig holds deployment-level settings.
type DeployConfig struct {
	Release string `toml:"release,omitempty"`
}

// TunnelConfig holds Cloudflare tunnel settings.
type TunnelConfig struct {
	Hostname  string   `toml:"hostname,omitempty"`
	Hostnames []string `toml:"hostnames,omitempty"`
}

// HealthCheckConfig defines health checking for a service.
type HealthCheckConfig struct {
	Type     string `toml:"type,omitempty"`     // "http", "tcp", "command"
	Path     string `toml:"path,omitempty"`     // HTTP path (for type=http)
	Port     int    `toml:"port,omitempty"`     // Port to check
	Command  string `toml:"command,omitempty"`  // Command to run (for type=command)
	Interval int    `toml:"interval,omitempty"` // Seconds between checks
	Timeout  int    `toml:"timeout,omitempty"`  // Seconds before timeout
	Retries  int    `toml:"retries,omitempty"`  // Number of retries
}

// ValidRuntimes lists supported runtime environments.
var ValidRuntimes = []string{"base", "node22", "node24", "python3.13"}

// ValidNetworkPolicies lists supported network policies.
var ValidNetworkPolicies = []string{"allow-all", "deny-all", "custom"}

// ValidAccessoryTypes lists supported accessory types.
var ValidAccessoryTypes = []string{"postgres", "redis", "mysql"}
