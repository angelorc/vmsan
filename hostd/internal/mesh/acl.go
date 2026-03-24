package mesh

import (
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strings"
)

const (
	// meshTableName is the nftables table for mesh ACLs.
	meshTableName = "vmsan_mesh"
	// meshChainName is the forward chain within the mesh table.
	meshChainName = "mesh_forward"
	// meshSetName is the concatenated set for O(1) ACL lookup.
	meshSetName = "mesh_acl"
)

// ACLEntry represents a single mesh access control rule.
type ACLEntry struct {
	SrcIP   string // Source VM mesh IP
	DstIP   string // Destination VM mesh IP
	DstPort uint16 // Destination port
	Proto   string // "tcp" or "udp"
}

// MeshFirewall manages nftables rules for mesh traffic.
// Default policy: deny all mesh traffic. Only explicitly allowed connections pass.
type MeshFirewall struct {
	logger *slog.Logger
}

// NewMeshFirewall creates a new mesh firewall manager.
func NewMeshFirewall(logger *slog.Logger) *MeshFirewall {
	if logger == nil {
		logger = slog.Default()
	}
	return &MeshFirewall{logger: logger}
}

// Init creates the mesh nftables table with a forward chain scoped to mesh
// traffic and a concatenated set for ACL entries.
//
// Chain logic (policy accept — non-mesh traffic passes through):
//  1. ct state established,related → accept (return traffic)
//  2. dst in 10.90.0.0/16 AND tuple in ACL set → accept
//  3. dst in 10.90.0.0/16 → drop (mesh traffic not in ACL)
//  4. everything else → accept (policy, e.g. VM internet traffic)
func (f *MeshFirewall) Init() error {
	ruleset := fmt.Sprintf(`
table ip %s {
	set %s {
		type ipv4_addr . ipv4_addr . inet_proto . inet_service
		flags interval
	}

	chain %s {
		type filter hook forward priority filter; policy accept;
		ct state established,related accept
		ip daddr 10.90.0.0/16 ip saddr . ip daddr . meta l4proto . th dport @%s accept
		ip daddr 10.90.0.0/16 counter drop
	}
}
`, meshTableName, meshSetName, meshChainName, meshSetName)

	if err := f.applyRuleset(ruleset); err != nil {
		return err
	}

	// The host also has iptables FORWARD rules (from hostiptables.go) for
	// Docker compatibility. iptables and nftables evaluate FORWARD hooks
	// independently — both must accept for traffic to pass. Docker sets the
	// iptables FORWARD policy to DROP, so cross-VM mesh traffic (veth-h-N →
	// veth-h-M) gets dropped even though our nftables chain accepts it.
	// Add an iptables rule to let mesh traffic through; ACL enforcement
	// remains in the nftables chain above.
	if out, err := exec.Command("iptables", "-C", "FORWARD",
		"-d", "10.90.0.0/16", "-j", "ACCEPT").CombinedOutput(); err != nil {
		// Rule doesn't exist yet, add it at the top of the chain.
		if out, err := exec.Command("iptables", "-I", "FORWARD", "1",
			"-d", "10.90.0.0/16", "-j", "ACCEPT").CombinedOutput(); err != nil {
			f.logger.Debug("failed to add iptables mesh FORWARD rule", "error", err, "output", string(out))
		}
		_ = out
	}

	return nil
}

// AllowMesh adds ACL entries to permit specific mesh connections.
// Uses nftables concatenated sets for O(1) lookup performance.
func (f *MeshFirewall) AllowMesh(entries []ACLEntry) error {
	if len(entries) == 0 {
		return nil
	}

	var elements []string
	for _, e := range entries {
		if err := validateACLEntry(e); err != nil {
			return fmt.Errorf("invalid ACL entry: %w", err)
		}
		elements = append(elements, fmt.Sprintf("%s . %s . %s . %d", e.SrcIP, e.DstIP, e.Proto, e.DstPort))
	}

	cmd := fmt.Sprintf("add element ip %s %s { %s }", meshTableName, meshSetName, strings.Join(elements, ", "))
	f.logger.Debug("adding mesh ACL entries", "count", len(entries))
	return f.nftCmd(cmd)
}

// DenyAll removes all ACL entries, effectively blocking all mesh traffic
// (the chain policy is DROP).
func (f *MeshFirewall) DenyAll() error {
	f.logger.Debug("flushing all mesh ACL entries")
	return f.nftCmd(fmt.Sprintf("flush set ip %s %s", meshTableName, meshSetName))
}

// RemoveVM removes all ACL entries involving the given mesh IP (as source or destination).
// Since nftables sets don't support wildcard deletion, this lists current elements
// and removes matching ones.
func (f *MeshFirewall) RemoveVM(meshIP string) error {
	f.logger.Debug("removing mesh ACL entries for VM", "meshIp", meshIP)

	// List current set elements.
	out, err := exec.Command("nft", "-s", "list", "set", "ip", meshTableName, meshSetName).CombinedOutput()
	if err != nil {
		// Table may not exist yet; that's fine.
		if strings.Contains(string(out), "No such file") {
			return nil
		}
		return fmt.Errorf("list mesh ACL set: %w: %s", err, out)
	}

	// Parse and find entries matching this IP by exact field comparison.
	// Lines look like: "10.90.0.1 . 10.90.0.2 . tcp . 5432,"
	var toRemove []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ",")
		if !strings.Contains(line, " . ") {
			continue
		}
		// Split into fields and match exact IPs (src or dst).
		fields := strings.Split(line, " . ")
		if len(fields) < 2 {
			continue
		}
		srcIP := strings.TrimSpace(fields[0])
		dstIP := strings.TrimSpace(fields[1])
		if srcIP == meshIP || dstIP == meshIP {
			toRemove = append(toRemove, line)
		}
	}

	if len(toRemove) == 0 {
		return nil
	}

	cmd := fmt.Sprintf("delete element ip %s %s { %s }", meshTableName, meshSetName, strings.Join(toRemove, ", "))
	return f.nftCmd(cmd)
}

// Cleanup removes the entire mesh nftables table and iptables mesh rule.
func (f *MeshFirewall) Cleanup() error {
	f.logger.Debug("cleaning up mesh firewall table")

	// Remove iptables FORWARD rule for mesh traffic (best-effort).
	exec.Command("iptables", "-D", "FORWARD", "-d", "10.90.0.0/16", "-j", "ACCEPT").Run()

	err := f.nftCmd(fmt.Sprintf("delete table ip %s", meshTableName))
	if err != nil {
		// Ignore "No such file" — table may not exist.
		if strings.Contains(err.Error(), "No such file") {
			return nil
		}
		return err
	}
	return nil
}

// applyRuleset applies an nftables ruleset via nft -f stdin.
func (f *MeshFirewall) applyRuleset(ruleset string) error {
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(ruleset)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apply mesh ruleset: %w: %s", err, out)
	}
	return nil
}

// nftCmd runs a single nft command. The command string is split into
// separate arguments since exec.Command does not invoke a shell.
func (f *MeshFirewall) nftCmd(command string) error {
	args := strings.Fields(command)
	out, err := exec.Command("nft", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft %q: %w: %s", command, err, out)
	}
	return nil
}

// validateACLEntry checks that an ACL entry has valid fields.
func validateACLEntry(e ACLEntry) error {
	if net.ParseIP(e.SrcIP) == nil {
		return fmt.Errorf("invalid source IP: %s", e.SrcIP)
	}
	if net.ParseIP(e.DstIP) == nil {
		return fmt.Errorf("invalid destination IP: %s", e.DstIP)
	}
	if e.DstPort == 0 {
		return fmt.Errorf("destination port must be non-zero")
	}
	if e.Proto != "tcp" && e.Proto != "udp" {
		return fmt.Errorf("protocol must be \"tcp\" or \"udp\", got %q", e.Proto)
	}
	return nil
}
