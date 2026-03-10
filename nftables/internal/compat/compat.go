package compat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	types "github.com/angelorc/vmsan/nftables"
)

// CleanupLegacyIptables removes iptables rules left behind by vmsan 0.1.0 VMs.
//
// Strategy (Phase 1 implementation):
//   - Run `iptables-save` and grep for the VM's device names (tapDevice, vethHost,
//     vethGuest) and IP addresses (hostIp, guestIp).
//   - For each matching rule, construct the corresponding `iptables -D` command.
//   - Target both nat (PREROUTING, POSTROUTING) and filter (FORWARD, INPUT, OUTPUT) tables.
//   - If netnsName is set, run cleanup inside the namespace via `ip netns exec`.
//   - Best-effort: log warnings on failure, do not return errors for individual rule deletions.
//
// This is intentionally a device-grep sweep (not exact rule reconstruction) because
// the 0.1.0 iptables rule format may differ between kernel versions (e.g., `-m state
// --state` vs `-m conntrack --ctstate`). Grepping for device names is robust.
//
// This shim will be removed in 0.3.0 when backward compatibility with 0.1.0 is dropped.
func CleanupLegacyIptables(ctx context.Context, opts *types.CleanupOptions, executor IptablesExecutor) error {
	patterns := buildPatterns(opts)
	if len(patterns) == 0 {
		return nil
	}

	output, err := executor.Save(ctx, opts.NetNSName)
	if err != nil {
		return fmt.Errorf("iptables-save: %w", err)
	}

	rules := parseMatchingRules(output, patterns)
	deleteRules(ctx, rules, opts.NetNSName, executor)
	return nil
}

// buildPatterns collects non-empty device names and IP addresses to grep for.
func buildPatterns(opts *types.CleanupOptions) []string {
	var patterns []string
	for _, s := range []string{opts.TapDevice, opts.VethHost, opts.VethGuest, opts.HostIP, opts.GuestIP} {
		if s != "" {
			patterns = append(patterns, s)
		}
	}
	return patterns
}

// iptablesRule holds a parsed rule line and its enclosing table name.
type iptablesRule struct {
	table string // e.g. "filter", "nat"
	args  string // e.g. "FORWARD -i fhvm0 -j ACCEPT" (the -A prefix stripped)
}

// parseMatchingRules scans iptables-save output and returns rules that mention
// any of the given patterns. It tracks the current table via *tablename headers.
func parseMatchingRules(output string, patterns []string) []iptablesRule {
	var (
		currentTable string
		rules        []iptablesRule
	)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		// Track table context: lines like "*filter" or "*nat".
		if strings.HasPrefix(line, "*") {
			currentTable = line[1:]
			continue
		}

		// Only process rule lines (start with -A).
		if !strings.HasPrefix(line, "-A ") {
			continue
		}

		// Check if any pattern appears in this rule line.
		if !containsAny(line, patterns) {
			continue
		}

		// Strip the "-A " prefix; keep "CHAIN_NAME ...rest".
		rules = append(rules, iptablesRule{
			table: currentTable,
			args:  line[3:],
		})
	}

	return rules
}

// deleteRules runs `iptables -t <table> -D <args>` for each rule.
// Deletion failures are logged but do not stop processing.
func deleteRules(ctx context.Context, rules []iptablesRule, netns string, executor IptablesExecutor) {
	for _, r := range rules {
		if err := deleteRule(ctx, r, netns, executor); err != nil {
			slog.WarnContext(ctx, "failed to delete iptables rule",
				"table", r.table,
				"rule", r.args,
				"error", err)
		}
	}
}

// deleteRule executes a single iptables -D command.
// If netns is non-empty, the command runs inside that network namespace.
func deleteRule(ctx context.Context, r iptablesRule, netns string, executor IptablesExecutor) error {
	// Build: iptables -t <table> -D <chain> <rest...>
	args := []string{"-t", r.table, "-D"}
	args = append(args, strings.Fields(r.args)...)

	// If namespace is specified, prepend ip netns exec to run inside the namespace
	if netns != "" {
		args = append([]string{"ip", "netns", "exec", netns, "iptables"}, args...)
		_, err := executor.Execute(ctx, args...)
		return err
	}

	_, err := executor.Execute(ctx, args...)
	return err
}

// containsAny reports whether s contains any of the given patterns as whole tokens.
// Uses word-boundary matching to prevent partial matches — e.g., "10.0.0.1"
// must not match a rule containing "10.0.0.10" or "10.0.0.100", and "tap0"
// must not match "tap0-old" or "tap0_backup".
//
// A pattern is considered to match if it appears as a complete token bounded by:
// - Start/end of string
// - Whitespace
// - Common delimiters: / : , - _ ( )
func containsAny(s string, patterns []string) bool {
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
