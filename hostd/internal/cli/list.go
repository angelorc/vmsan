package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/gwclient"
	"github.com/angelorc/vmsan/hostd/internal/output"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all VMs",
	RunE:    runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, _ []string) error {
	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer gw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := gw.Status(ctx)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	if len(resp.List) == 0 {
		if jsonOutput {
			fmt.Println(`{"count":0,"vms":[]}`)
		} else {
			fmt.Println("No VMs found.")
		}
		return nil
	}

	if jsonOutput {
		vms := make([]map[string]any, 0, len(resp.List))
		for _, vm := range resp.List {
			vms = append(vms, map[string]any{
				"id":        vm.VmId,
				"status":    vm.Status,
				"runtime":   vm.Runtime,
				"vcpus":     vm.Vcpus,
				"memMib":    vm.MemMib,
				"hostIp":    vm.HostIp,
				"guestIp":   vm.GuestIp,
				"meshIp":    vm.MeshIp,
				"createdAt": vm.CreatedAt,
				"timeoutAt": vm.TimeoutAt,
				"project":   vm.Project,
				"service":   vm.Service,
			})
		}
		data, _ := json.MarshalIndent(map[string]any{
			"count": len(resp.List),
			"vms":   vms,
		}, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Build table
	headers := []string{"ID", "STATUS", "RUNTIME", "VCPUS", "MEMORY", "HOST IP", "GUEST IP", "CREATED", "TIMEOUT"}
	var rows [][]string
	for _, vm := range resp.List {
		created := vm.CreatedAt
		if t, err := time.Parse(time.RFC3339, vm.CreatedAt); err == nil {
			created = output.TimeAgo(t)
		}
		timeout := "-"
		if vm.TimeoutAt != "" {
			if t, err := time.Parse(time.RFC3339, vm.TimeoutAt); err == nil {
				timeout = output.TimeRemaining(t)
			}
		}
		rows = append(rows, []string{
			vm.VmId,
			output.StatusColor(vm.Status),
			vm.Runtime,
			fmt.Sprintf("%d", vm.Vcpus),
			fmt.Sprintf("%d MiB", vm.MemMib),
			vm.HostIp,
			vm.GuestIp,
			created,
			timeout,
		})
	}

	output.PrintTable(os.Stdout, headers, rows)
	return nil
}
