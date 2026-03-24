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

var stopCmd = &cobra.Command{
	Use:   "stop <vmId> [vmId...]",
	Short: "Stop one or more running VMs",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(_ *cobra.Command, args []string) error {
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

	type result struct {
		VmID    string `json:"vmId"`
		Success bool   `json:"success"`
		Message string `json:"message,omitempty"`
	}
	var results []result
	hasErrors := false

	for _, id := range vmIDs {
		state, err := store.Load(id)
		if err != nil {
			results = append(results, result{VmID: id, Success: false, Message: err.Error()})
			hasErrors = true
			continue
		}

		if state.Status == "stopped" {
			if !jsonOutput {
				fmt.Printf("%s is already stopped\n", id)
			}
			results = append(results, result{VmID: id, Success: true, Message: "already stopped"})
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err = gw.FullStopVM(ctx, &vmsanv1.FullStopVMRequest{
			VmId:          id,
			Slot:          int32(slotFromState(state)),
			Pid:           int32(derefInt(state.PID)),
			NetNsName:     state.Network.NetNSName,
			SocketPath:    state.APISocket,
			JailerBaseDir: p.JailerBaseDir,
		})
		cancel()

		if err != nil {
			if !jsonOutput {
				fmt.Printf("Failed to stop %s: %v\n", id, err)
			}
			results = append(results, result{VmID: id, Success: false, Message: err.Error()})
			hasErrors = true
			continue
		}

		// Update local state
		_ = store.Update(id, func(s *vmstate.VmState) {
			s.Status = "stopped"
			s.PID = nil
		})

		if !jsonOutput {
			fmt.Printf("Stopped %s\n", id)
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
		return fmt.Errorf("some VMs failed to stop")
	}
	return nil
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
