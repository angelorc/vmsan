package firewall

import (
	"fmt"
	"runtime"

	"github.com/google/nftables"

	types "github.com/angelorc/vmsan/nftables"
	"github.com/angelorc/vmsan/nftables/internal/netns"
)

// Teardown removes the per-VM nftables table and host bypass chains.
//
// Phase 1: Delete vmsan_<vmId> table in the VM's network namespace.
// Deleting the table atomically removes all its chains and rules.
//
// Phase 2: Remove input_vm_<vmId> and output_vm_<vmId> chains from vmsan_host.
// If no chains remain in vmsan_host, the table itself is removed.
func Teardown(config types.TeardownConfig) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := deleteVMTable(config); err != nil {
		return err
	}

	return teardownHostBypass(config.VMId)
}

// deleteVMTable removes the per-VM nftables table from the network namespace.
func deleteVMTable(config types.TeardownConfig) error {
	c, cleanup, err := netns.NewConn(config.NetNSName)
	if err != nil {
		return err
	}
	defer cleanup()

	name := tableName(config.VMId)
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
func teardownHostBypass(vmId string) error {
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
