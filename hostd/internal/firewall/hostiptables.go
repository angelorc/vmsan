package firewall

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
)

// addHostIptables adds host-side FORWARD, MASQUERADE, and DNAT rules via
// iptables. These must use iptables (not nftables) to coexist with Docker's
// iptables-nft backend, which may set a FORWARD policy of DROP that a
// separate nftables chain cannot override.
func addHostIptables(ctx context.Context, opts *SetupOptions, executor IptablesExecutor) error {
	fwd := fwdDevice(opts)
	subnet := opts.VmIP + "/30"

	// MASQUERADE outbound traffic from guest subnet
	if _, err := executor.Execute(ctx, "-t", "nat", "-A", "POSTROUTING",
		"-s", subnet, "-o", opts.HostIface,
		"-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("masquerade: %w", err)
	}

	// FORWARD: guest -> internet
	if _, err := executor.Execute(ctx, "-A", "FORWARD",
		"-i", fwd, "-o", opts.HostIface,
		"-s", subnet, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("forward outbound: %w", err)
	}

	// FORWARD: internet -> guest (established/related only)
	if _, err := executor.Execute(ctx, "-A", "FORWARD",
		"-i", opts.HostIface, "-o", fwd,
		"-d", subnet,
		"-m", "state", "--state", "RELATED,ESTABLISHED",
		"-j", "ACCEPT"); err != nil {
		return fmt.Errorf("forward inbound: %w", err)
	}

	// DNAT for published ports
	if !opts.SkipDNAT {
		for _, pp := range opts.PublishedPorts {
			proto := pp.Protocol
			if proto == "" {
				proto = "tcp"
			}
			gip := opts.VmIP
			if pp.GuestIP != "" {
				gip = pp.GuestIP
			}

			// PREROUTING DNAT
			if _, err := executor.Execute(ctx, "-t", "nat", "-A", "PREROUTING",
				"-i", opts.HostIface,
				"-p", proto,
				"--dport", fmt.Sprintf("%d", pp.HostPort),
				"-j", "DNAT",
				"--to-destination", fmt.Sprintf("%s:%d", gip, pp.GuestPort)); err != nil {
				return fmt.Errorf("dnat port %d: %w", pp.HostPort, err)
			}

			// FORWARD to allow DNATed traffic through - restrict to DNAT path
			// Only accept packets entering on host interface and leaving via VM device
			if _, err := executor.Execute(ctx, "-A", "FORWARD",
				"-i", opts.HostIface,
				"-o", fwd,
				"-p", proto,
				"-d", gip,
				"--dport", fmt.Sprintf("%d", pp.GuestPort),
				"-j", "ACCEPT"); err != nil {
				return fmt.Errorf("forward dnat port %d: %w", pp.GuestPort, err)
			}
		}
	}

	return nil
}

// removeHostIptables removes host-side iptables rules for a VM by grepping
// iptables-save output for the VM's device names and guest IP, then issuing
// corresponding -D commands. Best-effort: logs warnings but does not error.
// Also removes per-port DNAT rules when PublishedPorts are provided.
func removeHostIptables(ctx context.Context, opts *TeardownOptions, executor IptablesExecutor) error {
	dev := opts.TapDevice
	if opts.VethHost != "" {
		dev = opts.VethHost
	}

	var patterns []string
	if dev != "" {
		patterns = append(patterns, dev)
	}
	if opts.GuestIP != "" {
		patterns = append(patterns, opts.GuestIP)
		// iptables normalizes e.g. 198.19.0.2/30 to 198.19.0.0/30.
		// Add the network address so MASQUERADE rules are matched too.
		if ip := net.ParseIP(opts.GuestIP); ip != nil {
			ip4 := ip.To4()
			if ip4 != nil {
				netIP := net.IP{ip4[0], ip4[1], ip4[2], ip4[3] & 0xFC}
				patterns = append(patterns, netIP.String())
			}
		}
	}

	// Add per-port guest IPs to patterns for DNAT rule cleanup
	for _, pp := range opts.PublishedPorts {
		if pp.GuestIP != "" {
			patterns = append(patterns, pp.GuestIP)
		}
	}

	if len(patterns) == 0 {
		return nil
	}

	removeMatchingRules(ctx, "filter", patterns, executor)
	removeMatchingRules(ctx, "nat", patterns, executor)
	return nil
}

// removeMatchingRules lists rules in the given iptables table, finds lines
// matching any pattern, and deletes them.
func removeMatchingRules(ctx context.Context, table string, patterns []string, executor IptablesExecutor) {
	out, err := executor.Execute(ctx, "-t", table, "-S")
	if err != nil {
		slog.WarnContext(ctx, "failed to list iptables rules", "table", table, "error", err)
		return
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-A ") {
			continue
		}
		if !matchesAny(line, patterns) {
			continue
		}
		// Convert -A to -D
		args := []string{"-t", table, "-D"}
		args = append(args, strings.Fields(line[3:])...)
		if _, err := executor.Execute(ctx, args...); err != nil {
			slog.WarnContext(ctx, "failed to delete iptables rule",
				"table", table,
				"rule", line[3:],
				"error", err)
		}
	}
}

// matchesAny reports whether s contains any of the given patterns as whole tokens.
// Uses word-boundary matching to prevent partial matches (e.g., "10.0.0.1" must
// not match "10.0.0.10", and "tap0" must not match "tap0-old").
//
// A pattern is considered to match if it appears as a complete token bounded by:
// - Start/end of string
// - Whitespace
// - Common delimiters: / : , - _ ( )
func matchesAny(s string, patterns []string) bool {
	for _, p := range patterns {
		if patternMatches(s, p) {
			return true
		}
	}
	return false
}

// patternMatches checks if pattern p appears as a complete token in s.
// It handles cases like "10.0.0.1" matching "10.0.0.1/32" but not "10.0.0.10".
func patternMatches(s, p string) bool {
	// Find all occurrences of the pattern
	idx := 0
	for {
		i := strings.Index(s[idx:], p)
		if i == -1 {
			return false
		}
		i += idx // adjust for offset

		// Check left boundary
		leftOK := i == 0 || isBoundary(s[i-1])
		// Check right boundary
		rightBound := i + len(p)
		rightOK := rightBound >= len(s) || isBoundary(s[rightBound])

		if leftOK && rightOK {
			return true
		}

		idx = i + 1 // continue searching
	}
}

// isBoundary reports whether c is a valid token boundary character.
func isBoundary(c byte) bool {
	return c == ' ' || c == '\t' || c == '/' || c == ':' || c == ',' ||
		c == '-' || c == '_' || c == '(' || c == ')' || c == '[' || c == ']'
}

// fwdDevice returns the device used for FORWARD rules: vethHost in netns mode,
// tapDevice otherwise.
func fwdDevice(config *SetupOptions) string {
	if config.VethHost != "" {
		return config.VethHost
	}
	return config.TapDevice
}
