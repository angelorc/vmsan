package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	vmsanv1 "github.com/angelorc/vmsan/hostd/gen/vmsan/v1"
	"github.com/angelorc/vmsan/hostd/internal/gwclient"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/vmstate"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:     "remove <vmId> [vmId...]",
	Aliases: []string{"rm"},
	Short:   "Remove one or more VMs (stops if running with --force)",
	Args:    cobra.MinimumNArgs(1),
	RunE:    runRemove,
}

func init() {
	removeCmd.Flags().BoolP("force", "f", false, "Force removal of running VMs (stops them first)")
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")

	p := paths.Resolve()
	store := vmstate.NewStore(p.VmsDir)

	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer gw.Close()

	// Deduplicate
	seen := make(map[string]bool)
	var vmIDs []string
	for _, id := range args {
		if !seen[id] {
			seen[id] = true
			vmIDs = append(vmIDs, id)
		}
	}

	// Validate all IDs exist
	for _, id := range vmIDs {
		if _, err := store.Load(id); err != nil {
			return fmt.Errorf("VM %s not found", id)
		}
	}

	// Block removal of non-stopped VMs unless --force
	if !force {
		var running []string
		for _, id := range vmIDs {
			state, _ := store.Load(id)
			if state != nil && state.Status != "stopped" {
				running = append(running, id)
			}
		}
		if len(running) > 0 {
			return fmt.Errorf("cannot remove running VM(s): %v. Stop them first or use --force (-f)", running)
		}
	}

	type result struct {
		VmID    string `json:"vmId"`
		Success bool   `json:"success"`
		Message string `json:"message,omitempty"`
	}
	var results []result
	hasErrors := false

	for _, id := range vmIDs {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := gw.DeleteVM(ctx, &vmsanv1.DeleteVMRequest{
			VmId:          id,
			Force:         force,
			JailerBaseDir: p.JailerBaseDir,
		})
		cancel()

		if err != nil {
			if !jsonOutput {
				fmt.Printf("Failed to remove %s: %v\n", id, err)
			}
			results = append(results, result{VmID: id, Success: false, Message: err.Error()})
			hasErrors = true
			continue
		}

		// Delete local state
		_ = store.Delete(id)

		if !jsonOutput {
			fmt.Printf("Removed %s\n", id)
		}
		results = append(results, result{VmID: id, Success: true})
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(map[string]any{
			"results": results,
		}, "", "  ")
		fmt.Println(string(data))
	}

	if hasErrors {
		return fmt.Errorf("some VMs failed to remove")
	}
	return nil
}
