package compat

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
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
func CleanupLegacyIptables(cfg types.CleanupConfig) error {
	patterns := buildPatterns(cfg)
	if len(patterns) == 0 {
		return nil
	}

	output, err := runIptablesSave(cfg.NetNSName)
	if err != nil {
		return fmt.Errorf("iptables-save: %w", err)
	}

	rules := parseMatchingRules(output, patterns)
	deleteRules(rules, cfg.NetNSName)
	return nil
}

// buildPatterns collects non-empty device names and IP addresses to grep for.
func buildPatterns(cfg types.CleanupConfig) []string {
	var patterns []string
	for _, s := range []string{cfg.TapDevice, cfg.VethHost, cfg.VethGuest, cfg.HostIP, cfg.GuestIP} {
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

// runIptablesSave executes iptables-save, optionally inside a network namespace.
func runIptablesSave(netns string) (string, error) {
	var cmd *exec.Cmd
	if netns != "" {
		cmd = exec.Command("ip", "netns", "exec", netns, "iptables-save")
	} else {
		cmd = exec.Command("iptables-save")
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
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
// Deletion failures are logged to stderr but do not stop processing.
func deleteRules(rules []iptablesRule, netns string) {
	for _, r := range rules {
		if err := deleteRule(r, netns); err != nil {
			fmt.Fprintf(os.Stderr, "vmsan-nftables: warning: failed to delete iptables rule (table=%s): %s: %v\n", r.table, r.args, err)
		}
	}
}

// deleteRule executes a single iptables -D command.
func deleteRule(r iptablesRule, netns string) error {
	// Build: iptables -t <table> -D <chain> <rest...>
	args := []string{"-t", r.table, "-D"}
	args = append(args, strings.Fields(r.args)...)

	var cmd *exec.Cmd
	if netns != "" {
		nsArgs := []string{"netns", "exec", netns, "iptables"}
		nsArgs = append(nsArgs, args...)
		cmd = exec.Command("ip", nsArgs...)
	} else {
		cmd = exec.Command("iptables", args...)
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// containsAny reports whether s contains any of the given patterns as whole tokens.
// Uses regexp.QuoteMeta to safely escape patterns (IPs, device names) and wraps
// them with boundary assertions to prevent partial matches — e.g., "10.0.0.1"
// must not match a rule containing "10.0.0.10" or "10.0.0.100".
func containsAny(s string, patterns []string) bool {
	for _, p := range patterns {
		// Match pattern bounded by non-alphanumeric/non-dot chars (or string edges).
		// In iptables-save output, IPs are delimited by spaces, slashes, or colons.
		re := regexp.MustCompile(`(?:^|[^0-9a-zA-Z.])` + regexp.QuoteMeta(p) + `(?:[^0-9a-zA-Z.]|$)`)
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
