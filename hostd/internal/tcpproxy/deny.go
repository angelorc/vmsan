// Package tcpproxy provides per-VM SNI filtering via an in-process TCP proxy.
//
// It wraps inet.af/tcpproxy to intercept TLS connections from VMs, extract the
// SNI hostname, and either proxy allowed domains or deny (TCP RST) disallowed ones.
package tcpproxy

import (
	"log/slog"
	"net"
	"sync/atomic"

	libtcp "inet.af/tcpproxy"
)

// Compile-time check: DenyTarget implements tcpproxy.Target.
var _ libtcp.Target = (*DenyTarget)(nil)

// DenyTarget implements tcpproxy.Target by closing connections (TCP RST)
// and logging the denied domain.
type DenyTarget struct {
	vmId   string
	logger *slog.Logger
	denied atomic.Int64
}

// NewDenyTarget creates a DenyTarget that logs denied connections for the given VM.
func NewDenyTarget(vmId string, logger *slog.Logger) *DenyTarget {
	return &DenyTarget{
		vmId:   vmId,
		logger: logger,
	}
}

// HandleConn closes the connection immediately and logs the denied domain.
func (d *DenyTarget) HandleConn(conn net.Conn) {
	d.denied.Add(1)

	src := conn.RemoteAddr().String()
	host, _, err := net.SplitHostPort(src)
	if err != nil {
		host = src
	}

	// Extract SNI hostname from the tcpproxy.Conn wrapper.
	domain := ""
	if tc, ok := conn.(*libtcp.Conn); ok {
		domain = tc.HostName
	}

	d.logger.Info("sni_denied",
		slog.String("event", "sni_denied"),
		slog.String("domain", domain),
		slog.String("src", host),
		slog.String("vmId", d.vmId),
	)

	conn.Close()
}

// DeniedCount returns the total number of denied connections.
func (d *DenyTarget) DeniedCount() int64 {
	return d.denied.Load()
}
