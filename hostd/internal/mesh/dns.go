package mesh

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	// defaultDNSPort is the default port for the mesh DNS server.
	defaultDNSPort = 10052

	// meshDNSSuffix is the DNS suffix for mesh service discovery.
	meshDNSSuffix = "vmsan.internal."

	// dnsTTL is the TTL in seconds for all mesh DNS records.
	dnsTTL = 5
)

// DNSHandler is a lightweight DNS server for mesh service discovery.
// It resolves <service>.<project>.vmsan.internal queries to mesh IPs.
type DNSHandler struct {
	allocator        *Allocator
	port             int
	logger           *slog.Logger
	server           *dns.Server
	mu               sync.Mutex
	notifyStartedFunc func() // called once after server is listening
}

// NewDNSHandler creates a new mesh DNS handler.
// If port is 0, defaultDNSPort (10052) is used.
func NewDNSHandler(allocator *Allocator, port int, logger *slog.Logger) *DNSHandler {
	if port == 0 {
		port = defaultDNSPort
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &DNSHandler{
		allocator: allocator,
		port:      port,
		logger:    logger,
	}
}

// Start begins serving DNS queries. It blocks until the context is cancelled
// or Stop is called.
func (h *DNSHandler) Start(ctx context.Context) error {
	h.mu.Lock()

	mux := dns.NewServeMux()
	mux.HandleFunc(meshDNSSuffix, h.handleQuery)
	mux.HandleFunc(".", h.handleForward)

	h.server = &dns.Server{
		Addr:              fmt.Sprintf(":%d", h.port),
		Net:               "udp",
		Handler:           mux,
		NotifyStartedFunc: h.notifyStartedFunc,
	}
	h.mu.Unlock()

	h.logger.Info("mesh DNS server starting", "port", h.port, "suffix", meshDNSSuffix)

	// Shut down when context is cancelled.
	go func() {
		<-ctx.Done()
		h.Stop()
	}()

	if err := h.server.ListenAndServe(); err != nil {
		// Ignore errors from server shutdown.
		select {
		case <-ctx.Done():
			return nil
		default:
			return fmt.Errorf("mesh DNS server: %w", err)
		}
	}
	return nil
}

// Stop gracefully shuts down the DNS server.
func (h *DNSHandler) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.server == nil {
		return nil
	}

	h.logger.Info("mesh DNS server stopping")
	err := h.server.Shutdown()
	h.server = nil
	return err
}

// handleQuery processes incoming DNS queries for the vmsan.internal zone.
func (h *DNSHandler) handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	for _, q := range r.Question {
		switch q.Qtype {
		case dns.TypeA:
			h.handleA(msg, q)
		case dns.TypeSRV:
			h.handleSRV(msg, q)
		case dns.TypeTXT:
			h.handleTXT(msg, q)
		default:
			// Return empty response for unsupported types.
		}
	}

	if err := w.WriteMsg(msg); err != nil {
		h.logger.Debug("failed to write DNS response", "error", err.Error())
	}
}

// handleForward proxies non-vmsan DNS queries to an upstream resolver.
func (h *DNSHandler) handleForward(w dns.ResponseWriter, r *dns.Msg) {
	const upstream = "8.8.8.8:53"
	c := new(dns.Client)
	c.Timeout = 5 * time.Second
	resp, _, err := c.Exchange(r, upstream)
	if err != nil {
		h.logger.Debug("upstream DNS failed", "error", err.Error())
		msg := new(dns.Msg)
		msg.SetRcode(r, dns.RcodeServerFailure)
		w.WriteMsg(msg)
		return
	}
	w.WriteMsg(resp)
}

// handleA resolves A record queries: <service>.<project>.vmsan.internal.
func (h *DNSHandler) handleA(msg *dns.Msg, q dns.Question) {
	service, project, ok := parseServiceQuery(q.Name)
	if !ok {
		msg.Rcode = dns.RcodeNameError
		return
	}

	assignment, found := h.allocator.GetByService(project, service)
	if !found {
		msg.Rcode = dns.RcodeNameError
		return
	}

	ip := net.ParseIP(assignment.MeshIP)
	if ip == nil {
		h.logger.Debug("invalid mesh IP in allocation", "meshIp", assignment.MeshIP, "service", service)
		msg.Rcode = dns.RcodeServerFailure
		return
	}

	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    dnsTTL,
		},
		A: ip,
	})
}

// handleSRV resolves SRV queries: _<service>._tcp.<project>.vmsan.internal.
func (h *DNSHandler) handleSRV(msg *dns.Msg, q dns.Question) {
	service, project, port, ok := parseSRVQuery(q.Name)
	if !ok {
		msg.Rcode = dns.RcodeNameError
		return
	}

	assignment, found := h.allocator.GetByService(project, service)
	if !found {
		msg.Rcode = dns.RcodeNameError
		return
	}

	target := fmt.Sprintf("%s.%s.%s", service, project, meshDNSSuffix)

	msg.Answer = append(msg.Answer, &dns.SRV{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeSRV,
			Class:  dns.ClassINET,
			Ttl:    dnsTTL,
		},
		Priority: 0,
		Weight:   0,
		Port:     port,
		Target:   target,
	})

	// Include the A record as an additional section.
	ip := net.ParseIP(assignment.MeshIP)
	if ip != nil {
		msg.Extra = append(msg.Extra, &dns.A{
			Hdr: dns.RR_Header{
				Name:   target,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    dnsTTL,
			},
			A: ip,
		})
	}
}

// handleTXT resolves TXT queries: _services.<project>.vmsan.internal.
func (h *DNSHandler) handleTXT(msg *dns.Msg, q dns.Question) {
	project, ok := parseServicesQuery(q.Name)
	if !ok {
		msg.Rcode = dns.RcodeNameError
		return
	}

	assignments := h.allocator.ListByProject(project)
	if len(assignments) == 0 {
		msg.Rcode = dns.RcodeNameError
		return
	}

	var services []string
	for _, a := range assignments {
		if a.Service != "" {
			services = append(services, a.Service)
		}
	}

	if len(services) == 0 {
		msg.Rcode = dns.RcodeNameError
		return
	}

	msg.Answer = append(msg.Answer, &dns.TXT{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeTXT,
			Class:  dns.ClassINET,
			Ttl:    dnsTTL,
		},
		Txt: services,
	})
}

// parseServiceQuery extracts service and project from "<service>.<project>.vmsan.internal."
// Returns (service, project, ok).
func parseServiceQuery(name string) (string, string, bool) {
	// Remove the vmsan.internal. suffix
	if !strings.HasSuffix(name, "."+meshDNSSuffix) {
		return "", "", false
	}
	prefix := strings.TrimSuffix(name, "."+meshDNSSuffix)

	parts := strings.SplitN(prefix, ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	service := parts[0]
	project := parts[1]

	if service == "" || project == "" {
		return "", "", false
	}

	return service, project, true
}

// parseSRVQuery extracts service, project, and port from "_<service>._tcp.<project>.vmsan.internal."
// The port is extracted from a conventional SRV naming scheme; since we don't have port
// info in the query name, we default to 0 and let the caller fill it.
// Returns (service, project, port, ok).
func parseSRVQuery(name string) (string, string, uint16, bool) {
	if !strings.HasSuffix(name, "."+meshDNSSuffix) {
		return "", "", 0, false
	}
	prefix := strings.TrimSuffix(name, "."+meshDNSSuffix)

	// Expected format: _<service>._tcp.<project> or _<service>._udp.<project>
	parts := strings.SplitN(prefix, ".", 3)
	if len(parts) != 3 {
		return "", "", 0, false
	}

	servicePart := parts[0]
	protoPart := parts[1]
	project := parts[2]

	if !strings.HasPrefix(servicePart, "_") {
		return "", "", 0, false
	}
	service := strings.TrimPrefix(servicePart, "_")

	if protoPart != "_tcp" && protoPart != "_udp" {
		return "", "", 0, false
	}

	if service == "" || project == "" {
		return "", "", 0, false
	}

	// SRV port is not in the query name; return 0 as default.
	return service, project, 0, true
}

// parseServicesQuery extracts project from "_services.<project>.vmsan.internal."
// Returns (project, ok).
func parseServicesQuery(name string) (string, bool) {
	if !strings.HasSuffix(name, "."+meshDNSSuffix) {
		return "", false
	}
	prefix := strings.TrimSuffix(name, "."+meshDNSSuffix)

	parts := strings.SplitN(prefix, ".", 2)
	if len(parts) != 2 {
		return "", false
	}

	if parts[0] != "_services" {
		return "", false
	}

	project := parts[1]
	if project == "" {
		return "", false
	}

	return project, true
}
