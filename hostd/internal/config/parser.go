package config

import (
	"fmt"
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

// Known field sets for schema validation.
var (
	topLevelKeys   = map[string]bool{"project": true, "runtime": true, "build": true, "start": true, "services": true, "accessories": true, "deploy": true, "tunnel": true}
	serviceKeys    = map[string]bool{"runtime": true, "build": true, "start": true, "env": true, "depends_on": true, "connect_to": true, "service": true, "publish_ports": true, "memory": true, "vcpus": true, "disk": true, "network_policy": true, "allowed_domains": true, "health_check": true}
	accessoryKeys  = map[string]bool{"type": true, "version": true, "env": true}
	deployKeys     = map[string]bool{"release": true}
	tunnelKeys     = map[string]bool{"hostname": true, "hostnames": true}
	healthKeys     = map[string]bool{"type": true, "path": true, "port": true, "command": true, "interval": true, "timeout": true, "retries": true}
)

// ParseVmsanToml parses a TOML string into a VmsanToml struct.
func ParseVmsanToml(content string) (*VmsanToml, error) {
	var cfg VmsanToml
	if err := toml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("toml parse error: %w", err)
	}
	return &cfg, nil
}

// LoadVmsanToml loads and parses a vmsan.toml file.
func LoadVmsanToml(filePath string) (*VmsanToml, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}
	return ParseVmsanToml(string(data))
}

// IsMultiService returns true if the config uses [services] blocks.
func IsMultiService(cfg *VmsanToml) bool {
	return len(cfg.Services) > 0
}

// NormalizeToml converts flat single-service format to multi-service format.
// If already multi-service, returns the existing services map.
func NormalizeToml(cfg *VmsanToml) map[string]ServiceConfig {
	if IsMultiService(cfg) {
		return cfg.Services
	}
	// Single-service mode: create a "web" service from top-level fields
	svc := ServiceConfig{
		Runtime: cfg.Runtime,
		Build:   cfg.Build,
		Start:   cfg.Start,
	}
	return map[string]ServiceConfig{"web": svc}
}

// UnknownFieldsRaw parses TOML into a raw map for schema checking.
// Returns unknown top-level keys.
func UnknownFieldsRaw(content string) ([]string, error) {
	var raw map[string]any
	if err := toml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, err
	}
	var unknown []string
	for k := range raw {
		if !topLevelKeys[k] {
			unknown = append(unknown, k)
		}
	}
	return unknown, nil
}

// UnknownServiceFields returns unknown fields within a service config block.
func UnknownServiceFields(raw map[string]any) []string {
	var unknown []string
	for k := range raw {
		if !serviceKeys[k] {
			unknown = append(unknown, k)
		}
	}
	return unknown
}

// UnknownAccessoryFields returns unknown fields within an accessory config block.
func UnknownAccessoryFields(raw map[string]any) []string {
	var unknown []string
	for k := range raw {
		if !accessoryKeys[k] {
			unknown = append(unknown, k)
		}
	}
	return unknown
}

// UnknownDeployFields returns unknown fields within a deploy block.
func UnknownDeployFields(raw map[string]any) []string {
	var unknown []string
	for k := range raw {
		if !deployKeys[k] {
			unknown = append(unknown, k)
		}
	}
	return unknown
}

// UnknownTunnelFields returns unknown fields within a tunnel block.
func UnknownTunnelFields(raw map[string]any) []string {
	var unknown []string
	for k := range raw {
		if !tunnelKeys[k] {
			unknown = append(unknown, k)
		}
	}
	return unknown
}

// UnknownHealthCheckFields returns unknown fields within a health_check block.
func UnknownHealthCheckFields(raw map[string]any) []string {
	var unknown []string
	for k := range raw {
		if !healthKeys[k] {
			unknown = append(unknown, k)
		}
	}
	return unknown
}
