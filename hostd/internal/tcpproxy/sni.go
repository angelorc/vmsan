package tcpproxy

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	libtcp "inet.af/tcpproxy"
)

// VmPolicy holds the policy settings for per-VM SNI filtering.
type VmPolicy struct {
	VMId           string
	Policy         string   // "allow-all", "deny-all", "custom"
	AllowedDomains []string // used with "custom" policy
}

// SNIProxy manages a per-VM tcpproxy listener that filters TLS by SNI.
type SNIProxy struct {
	mu         sync.Mutex
	vmId       string
	policy     string
	domains    []string
	listenAddr string
	proxy      *libtcp.Proxy
	deny       *DenyTarget
	logger     *slog.Logger
	started    bool
}

// NewSNIProxy creates an SNIProxy for the given VM policy and listen address.
//
// The listenAddr should be in "host:port" format (e.g., "127.0.0.1:10443").
// The proxy is not started until Start is called.
func NewSNIProxy(policy VmPolicy, listenAddr string, logger *slog.Logger) *SNIProxy {
	return &SNIProxy{
		vmId:       policy.VMId,
		policy:     policy.Policy,
		domains:    policy.AllowedDomains,
		listenAddr: listenAddr,
		deny:       NewDenyTarget(policy.VMId, logger),
		logger:     logger,
	}
}

// Start creates the proxy routes and starts the proxy listener as a goroutine.
func (s *SNIProxy) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("sni proxy already started for vm %s", s.vmId)
	}

	s.proxy = s.buildProxy()
	if err := s.proxy.Start(); err != nil {
		return fmt.Errorf("sni proxy start for vm %s: %w", s.vmId, err)
	}

	s.started = true
	s.logger.Info("sni proxy started",
		slog.String("vmId", s.vmId),
		slog.String("policy", s.policy),
		slog.String("listen", s.listenAddr),
		slog.Int("allowedDomains", len(s.domains)),
	)
	return nil
}

// Close stops the proxy and releases resources.
func (s *SNIProxy) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	err := s.proxy.Close()
	s.started = false
	s.logger.Info("sni proxy stopped",
		slog.String("vmId", s.vmId),
		slog.Int64("totalDenied", s.deny.DeniedCount()),
	)
	return err
}

// UpdatePolicy rebuilds the proxy routes with a new policy.
// The proxy is stopped and restarted with the updated configuration.
func (s *SNIProxy) UpdatePolicy(policy VmPolicy) error {
	s.mu.Lock()
	wasStarted := s.started
	s.mu.Unlock()

	if wasStarted {
		if err := s.Close(); err != nil {
			return fmt.Errorf("sni proxy close for update: %w", err)
		}
	}

	s.mu.Lock()
	s.vmId = policy.VMId
	s.policy = policy.Policy
	s.domains = policy.AllowedDomains
	s.deny = NewDenyTarget(policy.VMId, s.logger)
	s.mu.Unlock()

	if wasStarted {
		return s.Start()
	}
	return nil
}

// DenyTarget returns the deny target for inspection (e.g., denied count).
func (s *SNIProxy) DenyTarget() *DenyTarget {
	return s.deny
}

// ListenAddr returns the configured listen address.
func (s *SNIProxy) ListenAddr() string {
	return s.listenAddr
}

// buildProxy constructs a tcpproxy.Proxy with routes based on the current policy.
func (s *SNIProxy) buildProxy() *libtcp.Proxy {
	var p libtcp.Proxy

	switch s.policy {
	case "allow-all":
		// Observation mode: log all SNI hostnames but proxy everything through.
		p.AddSNIRouteFunc(s.listenAddr, func(_ context.Context, sni string) (libtcp.Target, bool) {
			s.logger.Debug("sni_observed",
				slog.String("event", "sni_observed"),
				slog.String("domain", sni),
				slog.String("vmId", s.vmId),
			)
			return &libtcp.DialProxy{Addr: sni + ":443"}, true
		})

	case "deny-all":
		// Deny all connections.
		p.AddRoute(s.listenAddr, s.deny)

	case "custom":
		// Build a domain lookup from the allowlist. Use SNIRouteFunc
		// for a single matching pass that handles both exact and wildcard.
		allowed := buildDomainSet(s.domains)
		deny := s.deny
		p.AddSNIRouteFunc(s.listenAddr, func(_ context.Context, sni string) (libtcp.Target, bool) {
			if matchDomain(allowed, sni) {
				return &libtcp.DialProxy{Addr: sni + ":443"}, true
			}
			return deny, true
		})
		// Fallback for connections without SNI (non-TLS or broken ClientHello).
		p.AddRoute(s.listenAddr, s.deny)

	default:
		// Unknown policy: deny everything as a safety net.
		p.AddRoute(s.listenAddr, s.deny)
	}

	return &p
}

// domainSet holds exact matches and wildcard suffixes for fast domain lookup.
type domainSet struct {
	exact     map[string]bool
	wildcards []string // each entry is a suffix like ".example.com"
}

// buildDomainSet creates a domainSet from a list of allowed domains.
// Wildcard entries like "*.example.com" are stored as suffix ".example.com".
func buildDomainSet(domains []string) *domainSet {
	ds := &domainSet{
		exact: make(map[string]bool, len(domains)),
	}
	for _, d := range domains {
		if strings.HasPrefix(d, "*.") {
			// Store the suffix portion: "*.example.com" -> ".example.com"
			ds.wildcards = append(ds.wildcards, d[1:])
		} else {
			ds.exact[d] = true
		}
	}
	return ds
}

// matchDomain checks whether sni matches any entry in the domain set.
func matchDomain(ds *domainSet, sni string) bool {
	if ds.exact[sni] {
		return true
	}
	for _, suffix := range ds.wildcards {
		if strings.HasSuffix(sni, suffix) {
			return true
		}
	}
	return false
}

// MatchDomain is exported for testing. It checks whether sni matches
// any entry in a domain set built from the given domains.
func MatchDomain(domains []string, sni string) bool {
	return matchDomain(buildDomainSet(domains), sni)
}
