package firewall

import (
	"fmt"
	"runtime"

	types "github.com/angelorc/vmsan/nftables"
	"github.com/angelorc/vmsan/nftables/internal/netns"
)

// Verify checks whether the nftables table for a VM exists
// and reports information about its chains.
func Verify(config types.VerifyConfig) (*types.VerifyResult, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	c, cleanup, err := netns.NewConn(config.NetNSName)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	result := &types.VerifyResult{
		NftResult: types.NftResult{OK: true},
	}

	tName := tableName(config.VMId)
	table, err := findTable(c, tName)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	if table == nil {
		return result, nil
	}

	result.TableExists = true
	chains, err := c.ListChainsOfTableFamily(table.Family)
	if err != nil {
		return nil, fmt.Errorf("list chains: %w", err)
	}
	for _, ch := range chains {
		if ch.Table.Name == tName {
			result.ChainCount++
		}
	}

	return result, nil
}
