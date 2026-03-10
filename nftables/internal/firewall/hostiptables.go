package firewall

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"

	types "github.com/angelorc/vmsan/nftables"
)

// addHostIptables adds host-side FORWARD, MASQUERADE, and DNAT rules via
// iptables. These must use iptables (not nftables) to coexist with Docker's
// iptables-nft backend, which may set a FORWARD policy of DROP that a
// separate nftables chain cannot override.
func addHostIptables(config types.SetupConfig) error {
	fwd := fwdDevice(config)
	subnet := config.GuestIP + "/30"

	// MASQUERADE outbound traffic from guest subnet
	if err := ipt("-t", "nat", "-A", "POSTROUTING",
		"-s", subnet, "-o", config.DefaultInterface,
		"-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("masquerade: %w", err)
	}

	// FORWARD: guest -> internet
	if err := ipt("-A", "FORWARD",
		"-i", fwd, "-o", config.DefaultInterface,
		"-s", subnet, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("forward outbound: %w", err)
	}

	// FORWARD: internet -> guest (established/related only)
	if err := ipt("-A", "FORWARD",
		"-i", config.DefaultInterface, "-o", fwd,
		"-d", subnet,
		"-m", "state", "--state", "RELATED,ESTABLISHED",
		"-j", "ACCEPT"); err != nil {
		return fmt.Errorf("forward inbound: %w", err)
	}

	// DNAT for published ports
	if !config.SkipDNAT {
		for _, pp := range config.PublishedPorts {
			proto := pp.Protocol
			if proto == "" {
				proto = "tcp"
			}
			gip := config.GuestIP
			if pp.GuestIP != "" {
				gip = pp.GuestIP
			}

			// PREROUTING DNAT
			if err := ipt("-t", "nat", "-A", "PREROUTING",
				"-i", config.DefaultInterface,
				"-p", proto,
				"--dport", fmt.Sprintf("%d", pp.HostPort),
				"-j", "DNAT",
				"--to-destination", fmt.Sprintf("%s:%d", gip, pp.GuestPort)); err != nil {
				return fmt.Errorf("dnat port %d: %w", pp.HostPort, err)
			}

			// FORWARD to allow DNATed traffic through
			if err := ipt("-A", "FORWARD",
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
func removeHostIptables(config types.TeardownConfig) error {
	dev := config.TapDevice
	if config.VethHost != "" {
		dev = config.VethHost
	}

	var patterns []string
	if dev != "" {
		patterns = append(patterns, dev)
	}
	if config.GuestIP != "" {
		patterns = append(patterns, config.GuestIP)
		// iptables normalizes e.g. 198.19.0.2/30 to 198.19.0.0/30.
		// Add the network address so MASQUERADE rules are matched too.
		if ip := net.ParseIP(config.GuestIP); ip != nil {
			ip4 := ip.To4()
			if ip4 != nil {
				netIP := net.IP{ip4[0], ip4[1], ip4[2], ip4[3] & 0xFC}
				patterns = append(patterns, netIP.String())
			}
		}
	}
	if len(patterns) == 0 {
		return nil
	}

	removeMatchingRules("filter", patterns)
	removeMatchingRules("nat", patterns)
	return nil
}

// removeMatchingRules lists rules in the given iptables table, finds lines
// matching any pattern, and deletes them.
func removeMatchingRules(table string, patterns []string) {
	out, err := exec.Command("iptables", "-t", table, "-S").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "vmsan-nftables: warning: iptables -t %s -S: %v\n", table, err)
		return
	}

	for _, line := range strings.Split(string(out), "\n") {
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
		cmd := exec.Command("iptables", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "vmsan-nftables: warning: iptables -t %s -D %s: %v: %s\n",
				table, line[3:], err, strings.TrimSpace(string(out)))
		}
	}
}

// matchesAny reports whether s contains any of the given patterns as whole tokens.
// Uses word-boundary matching to prevent partial matches (e.g., "10.0.0.1" must
// not match "10.0.0.10").
func matchesAny(s string, patterns []string) bool {
	for _, p := range patterns {
		re := regexp.MustCompile(`(?:^|[^0-9a-zA-Z.])` + regexp.QuoteMeta(p) + `(?:[^0-9a-zA-Z.]|$)`)
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// fwdDevice returns the device used for FORWARD rules: vethHost in netns mode,
// tapDevice otherwise.
func fwdDevice(config types.SetupConfig) string {
	if config.VethHost != "" {
		return config.VethHost
	}
	return config.TapDevice
}

// ipt runs a single iptables command with the given arguments.
func ipt(args ...string) error {
	cmd := exec.Command("iptables", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
