package firewall

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/angelorc/vmsan/hostd/internal/firewall/netns"
)

// Verify checks whether the nftables table for a VM exists
// and reports information about its chains.
func Verify(ctx context.Context, opts *VerifyOptions) (*VerifyResult, error) {
	slog.DebugContext(ctx, "verifying firewall", "vm_id", opts.VMId, "netns", opts.NetNSName)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	c, err := netns.NewConn(ctx, opts.NetNSName)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	result := &VerifyResult{
		NftResult: NftResult{OK: true},
	}

	tName := tableName(opts.VMId)
	table, err := findTable(c.Conn, tName)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	if table == nil {
		return result, nil
	}

	result.TableExists = true
	chains, err := c.Conn.ListChainsOfTableFamily(table.Family)
	if err != nil {
		return nil, fmt.Errorf("list chains: %w", err)
	}
	for _, ch := range chains {
		if ch.Table.Name == tName {
			result.ChainCount++
		}
	}

	slog.DebugContext(ctx, "firewall verification complete",
		"vm_id", opts.VMId,
		"table_exists", result.TableExists,
		"chain_count", result.ChainCount)

	return result, nil
}
