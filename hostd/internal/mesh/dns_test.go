package mesh

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// findFreePort finds an available UDP port for testing.
func findFreePort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()
	return port
}

// setupDNS creates an allocator with test data and starts a DNS handler.
// Returns the port, allocator, and a cleanup function.
func setupDNS(t *testing.T) (int, *Allocator, func()) {
	t.Helper()
	port := findFreePort(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	alloc := NewAllocator("")
	alloc.Allocate("myproject", "vm-web", "web")
	alloc.Allocate("myproject", "vm-db", "postgres")
	alloc.Allocate("otherproject", "vm-api", "api")

	handler := NewDNSHandler(alloc, port, logger)
	ctx, cancel := context.WithCancel(context.Background())

	// Use NotifyStartedFunc to know when the server is actually listening.
	ready := make(chan struct{})
	handler.notifyStartedFunc = func() {
		close(ready)
	}

	go func() {
		if err := handler.Start(ctx); err != nil {
			t.Logf("DNS server error: %v", err)
		}
	}()

	// Wait for the server to be ready.
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("DNS server did not start within 5 seconds")
	}

	cleanup := func() {
		cancel()
		handler.Stop()
	}
	return port, alloc, cleanup
}

func queryDNS(port int, name string, qtype uint16) (*dns.Msg, error) {
	c := new(dns.Client)
	c.Timeout = 2 * time.Second
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	r, _, err := c.Exchange(m, fmt.Sprintf("127.0.0.1:%d", port))
	return r, err
}

func TestDNSARecord(t *testing.T) {
	port, _, cleanup := setupDNS(t)
	defer cleanup()

	r, err := queryDNS(port, "web.myproject.vmsan.internal.", dns.TypeA)
	if err != nil {
		t.Fatalf("DNS query: %v", err)
	}

	if r.Rcode != dns.RcodeSuccess {
		t.Fatalf("DNS rcode = %d, want %d (success)", r.Rcode, dns.RcodeSuccess)
	}
	if len(r.Answer) != 1 {
		t.Fatalf("answer count = %d, want 1", len(r.Answer))
	}

	a, ok := r.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("answer is %T, want *dns.A", r.Answer[0])
	}
	if a.A.String() != "10.90.0.1" {
		t.Errorf("A record = %s, want 10.90.0.1", a.A.String())
	}
	if a.Hdr.Ttl != dnsTTL {
		t.Errorf("TTL = %d, want %d", a.Hdr.Ttl, dnsTTL)
	}
}

func TestDNSARecordPostgres(t *testing.T) {
	port, _, cleanup := setupDNS(t)
	defer cleanup()

	r, err := queryDNS(port, "postgres.myproject.vmsan.internal.", dns.TypeA)
	if err != nil {
		t.Fatalf("DNS query: %v", err)
	}

	if r.Rcode != dns.RcodeSuccess {
		t.Fatalf("DNS rcode = %d, want success", r.Rcode)
	}
	if len(r.Answer) != 1 {
		t.Fatalf("answer count = %d, want 1", len(r.Answer))
	}

	a := r.Answer[0].(*dns.A)
	if a.A.String() != "10.90.0.2" {
		t.Errorf("A record = %s, want 10.90.0.2", a.A.String())
	}
}

func TestDNSNXDOMAINMissingService(t *testing.T) {
	port, _, cleanup := setupDNS(t)
	defer cleanup()

	r, err := queryDNS(port, "nonexistent.myproject.vmsan.internal.", dns.TypeA)
	if err != nil {
		t.Fatalf("DNS query: %v", err)
	}

	if r.Rcode != dns.RcodeNameError {
		t.Errorf("DNS rcode = %d, want %d (NXDOMAIN)", r.Rcode, dns.RcodeNameError)
	}
}

func TestDNSProjectScoping(t *testing.T) {
	port, _, cleanup := setupDNS(t)
	defer cleanup()

	// Query for a service in a different project should return NXDOMAIN.
	r, err := queryDNS(port, "web.otherproject.vmsan.internal.", dns.TypeA)
	if err != nil {
		t.Fatalf("DNS query: %v", err)
	}

	if r.Rcode != dns.RcodeNameError {
		t.Errorf("cross-project query: rcode = %d, want %d (NXDOMAIN)", r.Rcode, dns.RcodeNameError)
	}
}

func TestDNSSRVRecord(t *testing.T) {
	port, _, cleanup := setupDNS(t)
	defer cleanup()

	r, err := queryDNS(port, "_web._tcp.myproject.vmsan.internal.", dns.TypeSRV)
	if err != nil {
		t.Fatalf("DNS query: %v", err)
	}

	if r.Rcode != dns.RcodeSuccess {
		t.Fatalf("DNS rcode = %d, want success", r.Rcode)
	}
	if len(r.Answer) != 1 {
		t.Fatalf("answer count = %d, want 1", len(r.Answer))
	}

	srv, ok := r.Answer[0].(*dns.SRV)
	if !ok {
		t.Fatalf("answer is %T, want *dns.SRV", r.Answer[0])
	}
	if srv.Target != "web.myproject.vmsan.internal." {
		t.Errorf("SRV target = %q, want %q", srv.Target, "web.myproject.vmsan.internal.")
	}

	// Should have an additional A record.
	if len(r.Extra) != 1 {
		t.Fatalf("extra count = %d, want 1", len(r.Extra))
	}
	a, ok := r.Extra[0].(*dns.A)
	if !ok {
		t.Fatalf("extra is %T, want *dns.A", r.Extra[0])
	}
	if a.A.String() != "10.90.0.1" {
		t.Errorf("extra A record = %s, want 10.90.0.1", a.A.String())
	}
}

func TestDNSTXTRecord(t *testing.T) {
	port, _, cleanup := setupDNS(t)
	defer cleanup()

	r, err := queryDNS(port, "_services.myproject.vmsan.internal.", dns.TypeTXT)
	if err != nil {
		t.Fatalf("DNS query: %v", err)
	}

	if r.Rcode != dns.RcodeSuccess {
		t.Fatalf("DNS rcode = %d, want success", r.Rcode)
	}
	if len(r.Answer) != 1 {
		t.Fatalf("answer count = %d, want 1", len(r.Answer))
	}

	txt, ok := r.Answer[0].(*dns.TXT)
	if !ok {
		t.Fatalf("answer is %T, want *dns.TXT", r.Answer[0])
	}

	// Should contain both "web" and "postgres".
	services := make(map[string]bool)
	for _, s := range txt.Txt {
		services[s] = true
	}
	if !services["web"] {
		t.Error("TXT record missing service: web")
	}
	if !services["postgres"] {
		t.Error("TXT record missing service: postgres")
	}
}

func TestDNSTXTNoServices(t *testing.T) {
	port, _, cleanup := setupDNS(t)
	defer cleanup()

	r, err := queryDNS(port, "_services.nonexistent.vmsan.internal.", dns.TypeTXT)
	if err != nil {
		t.Fatalf("DNS query: %v", err)
	}

	if r.Rcode != dns.RcodeNameError {
		t.Errorf("TXT for missing project: rcode = %d, want %d (NXDOMAIN)", r.Rcode, dns.RcodeNameError)
	}
}

func TestParseServiceQuery(t *testing.T) {
	tests := []struct {
		input       string
		wantService string
		wantProject string
		wantOK      bool
	}{
		{"web.myproject.vmsan.internal.", "web", "myproject", true},
		{"db.prod.vmsan.internal.", "db", "prod", true},
		{"vmsan.internal.", "", "", false},
		{"solo.vmsan.internal.", "", "", false},       // only one part before suffix
		{"too.many.parts.vmsan.internal.", "too", "many.parts", true}, // project can have dots
		{"", "", "", false},
	}

	for _, tt := range tests {
		service, project, ok := parseServiceQuery(tt.input)
		if ok != tt.wantOK {
			t.Errorf("parseServiceQuery(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			continue
		}
		if ok {
			if service != tt.wantService {
				t.Errorf("parseServiceQuery(%q) service = %q, want %q", tt.input, service, tt.wantService)
			}
			if project != tt.wantProject {
				t.Errorf("parseServiceQuery(%q) project = %q, want %q", tt.input, project, tt.wantProject)
			}
		}
	}
}

func TestParseSRVQuery(t *testing.T) {
	tests := []struct {
		input       string
		wantService string
		wantProject string
		wantOK      bool
	}{
		{"_web._tcp.myproject.vmsan.internal.", "web", "myproject", true},
		{"_db._udp.prod.vmsan.internal.", "db", "prod", true},
		{"web._tcp.myproject.vmsan.internal.", "", "", false}, // missing underscore on service
		{"_web.tcp.myproject.vmsan.internal.", "", "", false},  // missing underscore on proto
		{"_web._tcp.vmsan.internal.", "", "", false},           // missing project
	}

	for _, tt := range tests {
		service, project, _, ok := parseSRVQuery(tt.input)
		if ok != tt.wantOK {
			t.Errorf("parseSRVQuery(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			continue
		}
		if ok {
			if service != tt.wantService {
				t.Errorf("parseSRVQuery(%q) service = %q, want %q", tt.input, service, tt.wantService)
			}
			if project != tt.wantProject {
				t.Errorf("parseSRVQuery(%q) project = %q, want %q", tt.input, project, tt.wantProject)
			}
		}
	}
}

func TestParseServicesQuery(t *testing.T) {
	tests := []struct {
		input       string
		wantProject string
		wantOK      bool
	}{
		{"_services.myproject.vmsan.internal.", "myproject", true},
		{"_services.prod.vmsan.internal.", "prod", true},
		{"services.myproject.vmsan.internal.", "", false},  // missing underscore
		{"_services.vmsan.internal.", "", false},            // missing project
		{"_other.myproject.vmsan.internal.", "", false},     // wrong prefix
	}

	for _, tt := range tests {
		project, ok := parseServicesQuery(tt.input)
		if ok != tt.wantOK {
			t.Errorf("parseServicesQuery(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			continue
		}
		if ok && project != tt.wantProject {
			t.Errorf("parseServicesQuery(%q) project = %q, want %q", tt.input, project, tt.wantProject)
		}
	}
}
