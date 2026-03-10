// Package firewall manages per-VM nftables rulesets: setup, teardown, and verification.
package firewall

import (
	"fmt"
	"runtime"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"

	types "github.com/angelorc/vmsan/nftables"
	"github.com/angelorc/vmsan/nftables/internal/netns"
	"github.com/angelorc/vmsan/nftables/internal/rules"
)

// Setup creates the full nftables ruleset for a VM.
//
// Creates two sets of rules:
//  1. Per-VM table (vmsan_<vmId>) in the VM's network namespace with
//     prerouting (DNAT), forward (filter, policy drop), and postrouting (masquerade) chains.
//  2. Host bypass rules in the vmsan_host table (default namespace) with
//     input/output chains at priority -1 to bypass ufw/firewalld.
//
// Rule ordering in the forward chain follows the workflow specification:
//
//	1. ct state established,related accept
//	2. Interface forward accept (tap/veth)
//	3. DNS allow (configured resolvers) + DNS block (all others)
//	4. ICMP drop
//	5. UDP drop
//	6. DoT drop (TCP 853)
//	7. DoH drop (TCP 443 to known resolver IPs)
//	8. Cross-VM isolation (internal subnet drops)
//	9. Policy-specific rules (allow-all: accept, deny-all: nothing, custom: CIDR rules)
func Setup(config types.SetupConfig) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := setupVMTable(config); err != nil {
		return err
	}

	if config.GuestIP != "" {
		if err := setupHostBypass(config.VMId, config.GuestIP); err != nil {
			return fmt.Errorf("host bypass rules: %w", err)
		}
	}

	// Host-side iptables FORWARD/MASQUERADE/DNAT.
	// Required for Docker coexistence: nftables chains can't override
	// iptables-nft FORWARD DROP policy.
	if config.Policy != types.PolicyDenyAll {
		if err := addHostIptables(config); err != nil {
			return fmt.Errorf("host iptables: %w", err)
		}
	}

	return nil
}

// setupVMTable creates the per-VM nftables table in the VM's network namespace.
func setupVMTable(config types.SetupConfig) error {
	c, cleanup, err := netns.NewConn(config.NetNSName)
	if err != nil {
		return err
	}
	defer cleanup()

	table := c.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   tableName(config.VMId),
	})

	prerouting := rules.AddNATChain(c, table, "prerouting", nftables.ChainHookPrerouting, nftables.ChainPriorityNATDest)
	forward := rules.AddFilterChain(c, table, "forward", nftables.ChainHookForward, nftables.ChainPriorityFilter)
	postrouting := rules.AddNATChain(c, table, "postrouting", nftables.ChainHookPostrouting, nftables.ChainPriorityNATSource)

	fwd := rules.NewBuilder(c, table, forward)

	// 1. Allow established/related connections (MUST be first)
	fwd.Established()

	// 2. Interface forwarding (tap <-> veth, both directions)
	addInterfaceForwardRules(fwd, config)

	// 3. DNS: allow configured resolvers, block all others
	if err := fwd.DNSRules(config.DNSResolvers); err != nil {
		return err
	}

	// 4-7. Security rules (all policies)
	fwd.MatchProtoVerdict(unix.IPPROTO_ICMP, expr.VerdictDrop)
	fwd.MatchProtoVerdict(unix.IPPROTO_UDP, expr.VerdictDrop)
	fwd.MatchDstPort(unix.IPPROTO_TCP, 853, expr.VerdictDrop)
	if err := fwd.DoHDropRules(); err != nil {
		return err
	}

	// 8. Cross-VM isolation
	if err := fwd.CrossVMIsolation(); err != nil {
		return err
	}

	// 9. Policy-specific rules
	if err := addPolicyRules(fwd, config); err != nil {
		return err
	}

	// Prerouting chain (DNAT for published ports)
	if !config.SkipDNAT {
		if err := addPublishedPortRules(c, table, prerouting, config); err != nil {
			return err
		}
	}

	// Postrouting chain — intentionally empty in per-VM nftables namespace.
	// MASQUERADE/FORWARD/DNAT are handled on the HOST via iptables (see
	// addHostIptables) to coexist with Docker's iptables-nft backend.
	_ = postrouting

	return c.Flush()
}

// addInterfaceForwardRules allows bidirectional forwarding between
// the VM's tap device and veth interfaces.
func addInterfaceForwardRules(b *rules.Builder, config types.SetupConfig) {
	if config.TapDevice != "" && config.VethHost != "" {
		b.MatchIface(config.TapDevice, config.VethHost)
		b.MatchIface(config.VethHost, config.TapDevice)
	}
	if config.TapDevice != "" && config.VethGuest != "" {
		b.MatchIface(config.VethGuest, config.TapDevice)
		b.MatchIface(config.TapDevice, config.VethGuest)
	}
}

// addPolicyRules adds rules specific to the configured network policy.
func addPolicyRules(b *rules.Builder, config types.SetupConfig) error {
	switch config.Policy {
	case types.PolicyAllowAll:
		b.Accept()
	case types.PolicyDenyAll:
		// Nothing -- default chain policy is DROP
	case types.PolicyCustom:
		for _, cidr := range config.AllowedCIDRs {
			if err := b.MatchDstCIDR(cidr, expr.VerdictAccept); err != nil {
				return fmt.Errorf("allowed CIDR %s: %w", cidr, err)
			}
		}
		for _, cidr := range config.DeniedCIDRs {
			if err := b.MatchDstCIDR(cidr, expr.VerdictDrop); err != nil {
				return fmt.Errorf("denied CIDR %s: %w", cidr, err)
			}
		}
	default:
		return fmt.Errorf("unknown policy: %q", config.Policy)
	}
	return nil
}

// addPublishedPortRules adds DNAT rules for each published port.
func addPublishedPortRules(c *nftables.Conn, table *nftables.Table, chain *nftables.Chain, config types.SetupConfig) error {
	guestIP, err := rules.ParseIPv4(config.GuestIP)
	if err != nil {
		return fmt.Errorf("guest IP: %w", err)
	}

	b := rules.NewBuilder(c, table, chain)
	for _, pp := range config.PublishedPorts {
		dstIP := guestIP
		if pp.GuestIP != "" {
			ip, err := rules.ParseIPv4(pp.GuestIP)
			if err != nil {
				return fmt.Errorf("published port guest IP: %w", err)
			}
			dstIP = ip
		}

		proto := rules.ProtoNum(pp.Protocol)
		if proto == 0 {
			proto = unix.IPPROTO_TCP
		}

		b.DNAT(proto, pp.HostPort, dstIP, pp.GuestPort)
	}
	return nil
}
