package config

import (
	"testing"
)

func TestParseVmsanToml_MultiService(t *testing.T) {
	tomlStr := `
project = "myapp"

[services.web]
runtime = "node22"
build = "npm run build"
start = "npm start"
depends_on = ["db"]
publish_ports = [3000]
memory = 512
vcpus = 2
network_policy = "allow-all"

[services.worker]
runtime = "python3.13"
build = "pip install -r requirements.txt"
start = "python worker.py"
depends_on = ["db"]

[accessories.db]
type = "postgres"
version = "16"
`
	cfg, err := ParseVmsanToml(tomlStr)
	if err != nil {
		t.Fatalf("ParseVmsanToml() error: %v", err)
	}

	if cfg.Project != "myapp" {
		t.Errorf("Project = %q, want %q", cfg.Project, "myapp")
	}

	if len(cfg.Services) != 2 {
		t.Fatalf("len(Services) = %d, want 2", len(cfg.Services))
	}

	web, ok := cfg.Services["web"]
	if !ok {
		t.Fatal("missing service 'web'")
	}
	if web.Runtime != "node22" {
		t.Errorf("web.Runtime = %q, want %q", web.Runtime, "node22")
	}
	if web.Build != "npm run build" {
		t.Errorf("web.Build = %q, want %q", web.Build, "npm run build")
	}
	if web.Start != "npm start" {
		t.Errorf("web.Start = %q, want %q", web.Start, "npm start")
	}
	if len(web.DependsOn) != 1 || web.DependsOn[0] != "db" {
		t.Errorf("web.DependsOn = %v, want [db]", web.DependsOn)
	}
	if len(web.PublishPorts) != 1 || web.PublishPorts[0] != 3000 {
		t.Errorf("web.PublishPorts = %v, want [3000]", web.PublishPorts)
	}
	if web.Memory != 512 {
		t.Errorf("web.Memory = %d, want 512", web.Memory)
	}
	if web.Vcpus != 2 {
		t.Errorf("web.Vcpus = %d, want 2", web.Vcpus)
	}
	if web.NetworkPolicy != "allow-all" {
		t.Errorf("web.NetworkPolicy = %q, want %q", web.NetworkPolicy, "allow-all")
	}

	worker, ok := cfg.Services["worker"]
	if !ok {
		t.Fatal("missing service 'worker'")
	}
	if worker.Runtime != "python3.13" {
		t.Errorf("worker.Runtime = %q, want %q", worker.Runtime, "python3.13")
	}
	if worker.Start != "python worker.py" {
		t.Errorf("worker.Start = %q, want %q", worker.Start, "python worker.py")
	}

	db, ok := cfg.Accessories["db"]
	if !ok {
		t.Fatal("missing accessory 'db'")
	}
	if db.Type != "postgres" {
		t.Errorf("db.Type = %q, want %q", db.Type, "postgres")
	}
	if db.Version != "16" {
		t.Errorf("db.Version = %q, want %q", db.Version, "16")
	}
}

func TestParseVmsanToml_SingleService(t *testing.T) {
	tomlStr := `
runtime = "node22"
build = "npm run build"
start = "node server.js"
`
	cfg, err := ParseVmsanToml(tomlStr)
	if err != nil {
		t.Fatalf("ParseVmsanToml() error: %v", err)
	}

	if cfg.Runtime != "node22" {
		t.Errorf("Runtime = %q, want %q", cfg.Runtime, "node22")
	}
	if cfg.Build != "npm run build" {
		t.Errorf("Build = %q, want %q", cfg.Build, "npm run build")
	}
	if cfg.Start != "node server.js" {
		t.Errorf("Start = %q, want %q", cfg.Start, "node server.js")
	}
	if len(cfg.Services) != 0 {
		t.Errorf("len(Services) = %d, want 0", len(cfg.Services))
	}
}

func TestParseVmsanToml_Invalid(t *testing.T) {
	_, err := ParseVmsanToml("this is [[[ not valid toml {{{")
	if err == nil {
		t.Fatal("expected error for invalid TOML, got nil")
	}
}

func TestNormalizeToml_MultiService(t *testing.T) {
	cfg := &VmsanToml{
		Services: map[string]ServiceConfig{
			"api":    {Runtime: "node22", Start: "npm start"},
			"worker": {Runtime: "python3.13", Start: "python worker.py"},
		},
	}

	services := NormalizeToml(cfg)
	if len(services) != 2 {
		t.Fatalf("len(services) = %d, want 2", len(services))
	}
	if _, ok := services["api"]; !ok {
		t.Error("missing service 'api'")
	}
	if _, ok := services["worker"]; !ok {
		t.Error("missing service 'worker'")
	}
}

func TestNormalizeToml_SingleService(t *testing.T) {
	cfg := &VmsanToml{
		Runtime: "node22",
		Build:   "npm run build",
		Start:   "npm start",
	}

	services := NormalizeToml(cfg)
	if len(services) != 1 {
		t.Fatalf("len(services) = %d, want 1", len(services))
	}

	web, ok := services["web"]
	if !ok {
		t.Fatal("expected 'web' service in normalized output")
	}
	if web.Runtime != "node22" {
		t.Errorf("web.Runtime = %q, want %q", web.Runtime, "node22")
	}
	if web.Build != "npm run build" {
		t.Errorf("web.Build = %q, want %q", web.Build, "npm run build")
	}
	if web.Start != "npm start" {
		t.Errorf("web.Start = %q, want %q", web.Start, "npm start")
	}
}

func TestIsMultiService(t *testing.T) {
	tests := []struct {
		name string
		cfg  *VmsanToml
		want bool
	}{
		{
			name: "multi-service with services",
			cfg: &VmsanToml{
				Services: map[string]ServiceConfig{
					"web": {Start: "npm start"},
				},
			},
			want: true,
		},
		{
			name: "single-service no services block",
			cfg: &VmsanToml{
				Runtime: "node22",
				Start:   "npm start",
			},
			want: false,
		},
		{
			name: "empty services map",
			cfg: &VmsanToml{
				Services: map[string]ServiceConfig{},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMultiService(tt.cfg)
			if got != tt.want {
				t.Errorf("IsMultiService() = %v, want %v", got, tt.want)
			}
		})
	}
}
