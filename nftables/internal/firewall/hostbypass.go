package firewall

import (
	"fmt"

	"github.com/google/nftables"

	"github.com/angelorc/vmsan/nftables/internal/rules"
)

const hostTableName = "vmsan_host"

// setupHostBypass creates INPUT/OUTPUT chains in the vmsan_host table
// (default namespace) to accept traffic to/from the guest IP.
//
// These chains run at priority -1 (before ufw/firewalld at priority 0).
//
// LIMITATION: In nftables, each base chain evaluates independently.
// An "accept" verdict in this chain does NOT prevent a "drop" in another
// base chain (e.g., ufw/firewalld) at a later priority. If a host firewall
// actively blocks the guest IP range, users must add explicit allow rules
// to their firewall configuration. A mark-based bypass approach that fully
// overrides host firewall decisions is planned for a future release.
func setupHostBypass(vmId, guestIP string) error {
	ip4, err := rules.ParseIPv4(guestIP)
	if err != nil {
		return err
	}

	c, err := nftables.New()
	if err != nil {
		return fmt.Errorf("nftables conn (host): %w", err)
	}

	hostTable := c.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   hostTableName,
	})

	priorityBeforeUFW := nftables.ChainPriority(-1)
	policyAccept := nftables.ChainPolicyAccept

	// Accept traffic FROM guest IP (input)
	inputChain := c.AddChain(&nftables.Chain{
		Name:     fmt.Sprintf("input_vm_%s", vmId),
		Table:    hostTable,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookInput,
		Priority: &priorityBeforeUFW,
		Policy:   &policyAccept,
	})
	rules.NewBuilder(c, hostTable, inputChain).MatchIPAddr(ip4, rules.IPv4OffsetSrcAddr)

	// Accept traffic TO guest IP (output)
	outputChain := c.AddChain(&nftables.Chain{
		Name:     fmt.Sprintf("output_vm_%s", vmId),
		Table:    hostTable,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookOutput,
		Priority: &priorityBeforeUFW,
		Policy:   &policyAccept,
	})
	rules.NewBuilder(c, hostTable, outputChain).MatchIPAddr(ip4, rules.IPv4OffsetDstAddr)

	return c.Flush()
}
