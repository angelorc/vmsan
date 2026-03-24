package gateway

import (
	"testing"
)

func TestParseConnectTo(t *testing.T) {
	tests := []struct {
		input       string
		wantService string
		wantPort    uint16
		wantErr     bool
	}{
		{"postgres:5432", "postgres", 5432, false},
		{"redis:6379", "redis", 6379, false},
		{"web:80", "web", 80, false},
		{"service:1", "service", 1, false},
		{"service:65535", "service", 65535, false},
		{"invalid", "", 0, true},          // missing port
		{":5432", "", 0, true},            // empty service
		{"service:", "", 0, true},          // empty port
		{"service:0", "", 0, true},        // zero port
		{"service:abc", "", 0, true},      // non-numeric port
		{"service:99999", "", 0, true},    // port out of range
		{"service:-1", "", 0, true},       // negative port
	}

	for _, tt := range tests {
		service, port, err := parseConnectTo(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseConnectTo(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			if service != tt.wantService {
				t.Errorf("parseConnectTo(%q) service = %q, want %q", tt.input, service, tt.wantService)
			}
			if port != tt.wantPort {
				t.Errorf("parseConnectTo(%q) port = %d, want %d", tt.input, port, tt.wantPort)
			}
		}
	}
}

func TestNewMeshManager(t *testing.T) {
	m := NewMeshManager(nil, nil)
	if m == nil {
		t.Fatal("NewMeshManager returned nil")
	}
	if m.allocator == nil {
		t.Error("allocator is nil")
	}
	if m.dns == nil {
		t.Error("dns handler is nil")
	}
	if m.firewall == nil {
		t.Error("firewall is nil")
	}
}
