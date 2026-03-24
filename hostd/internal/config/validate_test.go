package config

import (
	"strings"
	"testing"
)

func TestValidateToml(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *VmsanToml
		wantErrors int
		wantField  string
		wantSubstr string
	}{
		{
			name: "valid config",
			cfg: &VmsanToml{
				Services: map[string]ServiceConfig{
					"web": {Runtime: "node22", Start: "npm start"},
				},
				Accessories: map[string]AccessoryConfig{
					"db": {Type: "postgres"},
				},
			},
			wantErrors: 0,
		},
		{
			name: "missing start command",
			cfg: &VmsanToml{
				Services: map[string]ServiceConfig{
					"web": {Runtime: "node22"},
				},
			},
			wantErrors: 1,
			wantField:  "services.web.start",
			wantSubstr: "missing a start command",
		},
		{
			name: "unknown runtime",
			cfg: &VmsanToml{
				Services: map[string]ServiceConfig{
					"web": {Runtime: "nod22", Start: "npm start"},
				},
			},
			wantErrors: 1,
			wantField:  "services.web.runtime",
			wantSubstr: "Did you mean",
		},
		{
			name: "circular dependency",
			cfg: &VmsanToml{
				Services: map[string]ServiceConfig{
					"a": {Start: "start-a", DependsOn: []string{"b"}},
					"b": {Start: "start-b", DependsOn: []string{"a"}},
				},
			},
			wantErrors: 1,
			wantField:  "depends_on",
			wantSubstr: "Circular dependency",
		},
		{
			name: "duplicate name",
			cfg: &VmsanToml{
				Services: map[string]ServiceConfig{
					"db": {Start: "start-db"},
				},
				Accessories: map[string]AccessoryConfig{
					"db": {Type: "postgres"},
				},
			},
			wantErrors: 1,
			wantField:  "services.db",
			wantSubstr: "Duplicate name",
		},
		{
			name: "invalid port",
			cfg: &VmsanToml{
				Services: map[string]ServiceConfig{
					"web": {Start: "npm start", PublishPorts: []int{99999}},
				},
			},
			wantErrors: 1,
			wantField:  "services.web.publish_ports",
			wantSubstr: "Invalid port",
		},
		{
			name: "missing dependency",
			cfg: &VmsanToml{
				Services: map[string]ServiceConfig{
					"web": {Start: "npm start", DependsOn: []string{"cache"}},
				},
			},
			wantErrors: 1,
			wantField:  "services.web.depends_on",
			wantSubstr: "not defined",
		},
		{
			name: "invalid network policy",
			cfg: &VmsanToml{
				Services: map[string]ServiceConfig{
					"web": {Start: "npm start", NetworkPolicy: "block-all"},
				},
			},
			wantErrors: 1,
			wantField:  "services.web.network_policy",
			wantSubstr: "Valid policies",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateToml(tt.cfg)
			if len(errs) != tt.wantErrors {
				t.Fatalf("ValidateToml() returned %d errors, want %d: %+v", len(errs), tt.wantErrors, errs)
			}
			if tt.wantErrors == 0 {
				return
			}
			// Check at least one error matches expected field and substring
			found := false
			for _, e := range errs {
				fieldMatch := tt.wantField == "" || e.Field == tt.wantField
				msgMatch := tt.wantSubstr == "" || strings.Contains(e.Message, tt.wantSubstr) || strings.Contains(e.Suggestion, tt.wantSubstr)
				if fieldMatch && msgMatch {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no error matching field=%q substr=%q in %+v", tt.wantField, tt.wantSubstr, errs)
			}
		})
	}
}
