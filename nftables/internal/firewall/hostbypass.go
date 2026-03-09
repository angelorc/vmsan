package firewall

import (
	"fmt"

	"github.com/google/nftables"

	"github.com/angelorc/vmsan/nftables/internal/rules"
)

const hostTableName = "vmsan_host"

// setupHostBypass creates INPUT/OUTPUT chains in the vmsan_host table
// (default namespace) to allow traffic to/from the guest IP.
// Priority -1 ensures these run before ufw/firewalld (priority 0).
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

	// Accept traffic FROM guest IP (input)
	inputChain := c.AddChain(&nftables.Chain{
		Name:     fmt.Sprintf("input_vm_%s", vmId),
		Table:    hostTable,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookInput,
		Priority: &priorityBeforeUFW,
	})
	rules.NewBuilder(c, hostTable, inputChain).MatchIPAddr(ip4, rules.IPv4OffsetSrcAddr)

	// Accept traffic TO guest IP (output)
	outputChain := c.AddChain(&nftables.Chain{
		Name:     fmt.Sprintf("output_vm_%s", vmId),
		Table:    hostTable,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookOutput,
		Priority: &priorityBeforeUFW,
	})
	rules.NewBuilder(c, hostTable, outputChain).MatchIPAddr(ip4, rules.IPv4OffsetDstAddr)

	return c.Flush()
}
