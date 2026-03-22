package tcpproxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewSNIProxy(t *testing.T) {
	logger := discardLogger()

	tests := []struct {
		name   string
		policy VmPolicy
		listen string
	}{
		{
			name: "allow-all policy",
			policy: VmPolicy{
				VMId:   "vm-allow",
				Policy: "allow-all",
			},
			listen: "127.0.0.1:10443",
		},
		{
			name: "deny-all policy",
			policy: VmPolicy{
				VMId:   "vm-deny",
				Policy: "deny-all",
			},
			listen: "127.0.0.1:10444",
		},
		{
			name: "custom policy",
			policy: VmPolicy{
				VMId:           "vm-custom",
				Policy:         "custom",
				AllowedDomains: []string{"example.com", "*.github.com"},
			},
			listen: "127.0.0.1:10445",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := NewSNIProxy(tt.policy, tt.listen, logger)
			if proxy == nil {
				t.Fatal("NewSNIProxy returned nil")
			}
			if proxy.vmId != tt.policy.VMId {
				t.Errorf("vmId = %q, want %q", proxy.vmId, tt.policy.VMId)
			}
			if proxy.policy != tt.policy.Policy {
				t.Errorf("policy = %q, want %q", proxy.policy, tt.policy.Policy)
			}
			if proxy.ListenAddr() != tt.listen {
				t.Errorf("ListenAddr() = %q, want %q", proxy.ListenAddr(), tt.listen)
			}
			if proxy.DenyTarget() == nil {
				t.Error("DenyTarget() returned nil")
			}
		})
	}
}

func TestMatchDomainExact(t *testing.T) {
	tests := []struct {
		domains []string
		sni     string
		want    bool
	}{
		{[]string{"example.com"}, "example.com", true},
		{[]string{"example.com"}, "other.com", false},
		{[]string{"example.com", "other.com"}, "other.com", true},
		{[]string{"example.com"}, "sub.example.com", false},
		{[]string{}, "example.com", false},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%v_%s", tt.domains, tt.sni)
		t.Run(name, func(t *testing.T) {
			got := MatchDomain(tt.domains, tt.sni)
			if got != tt.want {
				t.Errorf("MatchDomain(%v, %q) = %v, want %v", tt.domains, tt.sni, got, tt.want)
			}
		})
	}
}

func TestMatchDomainWildcard(t *testing.T) {
	tests := []struct {
		domains []string
		sni     string
		want    bool
	}{
		{[]string{"*.example.com"}, "foo.example.com", true},
		{[]string{"*.example.com"}, "bar.example.com", true},
		{[]string{"*.example.com"}, "deep.sub.example.com", true},
		{[]string{"*.example.com"}, "example.com", false},
		{[]string{"*.example.com"}, "notexample.com", false},
		{[]string{"*.example.com"}, "other.com", false},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%v_%s", tt.domains, tt.sni)
		t.Run(name, func(t *testing.T) {
			got := MatchDomain(tt.domains, tt.sni)
			if got != tt.want {
				t.Errorf("MatchDomain(%v, %q) = %v, want %v", tt.domains, tt.sni, got, tt.want)
			}
		})
	}
}

func TestMatchDomainMixed(t *testing.T) {
	domains := []string{"exact.com", "*.wildcard.org"}

	tests := []struct {
		sni  string
		want bool
	}{
		{"exact.com", true},
		{"foo.wildcard.org", true},
		{"bar.wildcard.org", true},
		{"wildcard.org", false},
		{"unknown.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.sni, func(t *testing.T) {
			got := MatchDomain(domains, tt.sni)
			if got != tt.want {
				t.Errorf("MatchDomain(%v, %q) = %v, want %v", domains, tt.sni, got, tt.want)
			}
		})
	}
}

func TestBuildDomainSet(t *testing.T) {
	ds := buildDomainSet([]string{"a.com", "*.b.com", "c.com", "*.d.org"})

	if len(ds.exact) != 2 {
		t.Errorf("exact count = %d, want 2", len(ds.exact))
	}
	if !ds.exact["a.com"] {
		t.Error("expected a.com in exact set")
	}
	if !ds.exact["c.com"] {
		t.Error("expected c.com in exact set")
	}
	if len(ds.wildcards) != 2 {
		t.Errorf("wildcard count = %d, want 2", len(ds.wildcards))
	}
}

func TestSNIProxyStartClose(t *testing.T) {
	logger := discardLogger()

	// Find a free port for the test.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	proxy := NewSNIProxy(VmPolicy{
		VMId:   "test-vm",
		Policy: "deny-all",
	}, addr, logger)

	// Start the proxy.
	if err := proxy.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Double start should fail.
	if err := proxy.Start(); err == nil {
		t.Error("expected error on double Start")
	}

	// Close the proxy.
	if err := proxy.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Double close is a no-op.
	if err := proxy.Close(); err != nil {
		t.Fatalf("double Close: %v", err)
	}
}

func TestSNIProxyDenyAllRejectsConnection(t *testing.T) {
	logger := discardLogger()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	proxy := NewSNIProxy(VmPolicy{
		VMId:   "test-deny",
		Policy: "deny-all",
	}, addr, logger)

	if err := proxy.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer proxy.Close()

	// Connect and send a TLS ClientHello. The proxy should close it.
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Attempt a TLS handshake — expect it to fail since the proxy closes the conn.
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         "evil.com",
		InsecureSkipVerify: true,
	})
	defer tlsConn.Close()

	// Set a deadline so we don't hang forever.
	tlsConn.SetDeadline(time.Now().Add(2 * time.Second))

	err = tlsConn.Handshake()
	if err == nil {
		t.Error("expected TLS handshake to fail, but it succeeded")
	}

	// Wait briefly for the deny target to process.
	time.Sleep(100 * time.Millisecond)

	if count := proxy.DenyTarget().DeniedCount(); count < 1 {
		t.Errorf("denied count = %d, want >= 1", count)
	}
}

func TestSNIProxyUpdatePolicy(t *testing.T) {
	logger := discardLogger()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	proxy := NewSNIProxy(VmPolicy{
		VMId:   "test-update",
		Policy: "allow-all",
	}, addr, logger)

	if err := proxy.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Update to deny-all while running.
	err = proxy.UpdatePolicy(VmPolicy{
		VMId:   "test-update",
		Policy: "deny-all",
	})
	if err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}

	// Verify the proxy was restarted with the new policy.
	if proxy.policy != "deny-all" {
		t.Errorf("policy = %q after update, want %q", proxy.policy, "deny-all")
	}

	proxy.Close()
}

func TestSNIProxyUpdatePolicyNotStarted(t *testing.T) {
	logger := discardLogger()

	proxy := NewSNIProxy(VmPolicy{
		VMId:   "test-no-start",
		Policy: "allow-all",
	}, "127.0.0.1:0", logger)

	// Update without starting — should just update fields.
	err := proxy.UpdatePolicy(VmPolicy{
		VMId:           "test-no-start",
		Policy:         "custom",
		AllowedDomains: []string{"example.com"},
	})
	if err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}

	if proxy.policy != "custom" {
		t.Errorf("policy = %q, want %q", proxy.policy, "custom")
	}
	if len(proxy.domains) != 1 || proxy.domains[0] != "example.com" {
		t.Errorf("domains = %v, want [example.com]", proxy.domains)
	}
}
