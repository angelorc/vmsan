package firewall

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/google/nftables"

	"github.com/angelorc/vmsan/hostd/internal/firewall/netns"
)

// Teardown removes the per-VM nftables table and host bypass chains.
//
// Phase 1: Delete vmsan_<vmId> table in the VM's network namespace.
// Deleting the table atomically removes all its chains and rules.
//
// Phase 2: Remove input_vm_<vmId> and output_vm_<vmId> chains from vmsan_host.
// If no chains remain in vmsan_host, the table itself is removed.
func Teardown(ctx context.Context, opts *TeardownOptions) error {
	slog.DebugContext(ctx, "tearing down firewall", "vm_id", opts.VMId, "netns", opts.NetNSName)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var errs []error
	if err := deleteVMTable(ctx, opts); err != nil {
		errs = append(errs, fmt.Errorf("delete VM table: %w", err))
	}
	if err := teardownHostBypass(ctx, opts.VMId); err != nil {
		errs = append(errs, fmt.Errorf("teardown host bypass: %w", err))
	}

	// Host-side iptables cleanup (best-effort)
	if opts.GuestIP != "" || opts.TapDevice != "" {
		executor := NewRealIptablesExecutor()
		if err := removeHostIptables(ctx, opts, executor); err != nil {
			errs = append(errs, fmt.Errorf("host iptables cleanup: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	slog.DebugContext(ctx, "firewall teardown complete", "vm_id", opts.VMId)
	return nil
}

// deleteVMTable removes the per-VM nftables table from the network namespace.
func deleteVMTable(ctx context.Context, opts *TeardownOptions) error {
	c, err := netns.NewConn(ctx, opts.NetNSName)
	if err != nil {
		return err
	}
	defer c.Close()

	name := tableName(opts.VMId)
	c.DelTable(&nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   name,
	})

	if err := c.Flush(); err != nil {
		return fmt.Errorf("delete table %s: %w", name, err)
	}
	return nil
}

// teardownHostBypass removes the per-VM chains from the vmsan_host table.
// If the table has no remaining chains, it is deleted entirely.
func teardownHostBypass(ctx context.Context, vmId string) error {
	c, err := nftables.New()
	if err != nil {
		return fmt.Errorf("nftables conn (host): %w", err)
	}

	hostTable, err := findTable(c, hostTableName)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}
	if hostTable == nil {
		return nil
	}

	inputName := fmt.Sprintf("input_vm_%s", vmId)
	outputName := fmt.Sprintf("output_vm_%s", vmId)

	chains, err := c.ListChainsOfTableFamily(nftables.TableFamilyIPv4)
	if err != nil {
		return fmt.Errorf("list chains: %w", err)
	}

	for _, ch := range chains {
		if ch.Table.Name != hostTableName {
			continue
		}
		if ch.Name == inputName || ch.Name == outputName {
			c.DelChain(ch)
		}
	}

	if err := c.Flush(); err != nil {
		return fmt.Errorf("delete host bypass chains: %w", err)
	}

	cleanupEmptyHostTable(c, hostTable)
	return nil
}

// cleanupEmptyHostTable removes the vmsan_host table if it has no remaining chains.
// This is best-effort -- errors are silently ignored because the table will be
// cleaned up on next VM teardown anyway.
func cleanupEmptyHostTable(c *nftables.Conn, table *nftables.Table) {
	chains, err := c.ListChainsOfTableFamily(nftables.TableFamilyIPv4)
	if err != nil {
		return
	}
	for _, ch := range chains {
		if ch.Table.Name == hostTableName {
			return
		}
	}
	c.DelTable(table)
	_ = c.Flush() // best-effort cleanup
}
