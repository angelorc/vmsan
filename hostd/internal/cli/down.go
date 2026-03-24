package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	vmsanv1 "github.com/angelorc/vmsan/hostd/gen/vmsan/v1"
	"github.com/angelorc/vmsan/hostd/internal/config"
	"github.com/angelorc/vmsan/hostd/internal/deploy"
	"github.com/angelorc/vmsan/hostd/internal/gwclient"
	"github.com/angelorc/vmsan/hostd/internal/output"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/vmstate"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop all services for the current project",
	RunE:  runDown,
}

func init() {
	f := downCmd.Flags()
	f.String("config", "vmsan.toml", "Path to vmsan.toml")
	f.Bool("destroy", false, "Also remove VMs and all data")
	f.Bool("force", false, "Skip confirmation prompt")
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, _ []string) error {
	cfgPath, _ := cmd.Flags().GetString("config")
	destroy, _ := cmd.Flags().GetBool("destroy")
	force, _ := cmd.Flags().GetBool("force")

	cfg, err := config.LoadVmsanToml(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	p := paths.Resolve()
	store := vmstate.NewStore(p.VmsDir)
	allVMs, err := store.List()
	if err != nil {
		return fmt.Errorf("list VMs: %w", err)
	}

	// Find VMs belonging to this project
	var projectVMs []*vmstate.VmState
	for _, vm := range allVMs {
		if vm.Project == cfg.Project {
			projectVMs = append(projectVMs, vm)
		}
	}

	if len(projectVMs) == 0 {
		fmt.Println("No VMs found for this project.")
		return nil
	}

	// Confirm destruction
	if destroy && !force {
		fmt.Printf("  This will destroy %d VM(s) and all their data. Continue? [y/N] ", len(projectVMs))
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("  Aborted.")
			return nil
		}
	}

	// Build reverse dependency order
	services := config.NormalizeToml(cfg)
	graph, err := deploy.BuildDependencyGraph(services, cfg.Accessories)
	if err != nil {
		// Fall back to unordered shutdown
		graph = &deploy.DependencyGraph{ReverseOrder: make([]string, 0)}
		for _, vm := range projectVMs {
			graph.ReverseOrder = append(graph.ReverseOrder, vm.Network.Service)
		}
	}

	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer gw.Close()

	type stopResult struct {
		Service string `json:"service"`
		VmID    string `json:"vmId"`
		Status  string `json:"status"`
		Error   string `json:"error,omitempty"`
	}
	var results []stopResult

	stopVM := func(vm *vmstate.VmState, svcName string) stopResult {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, stopErr := gw.FullStopVM(ctx, &vmsanv1.FullStopVMRequest{VmId: vm.ID})
		cancel()

		if stopErr != nil {
			return stopResult{Service: svcName, VmID: vm.ID, Status: "error", Error: stopErr.Error()}
		}

		_ = store.Update(vm.ID, func(s *vmstate.VmState) {
			s.Status = "stopped"
		})

		if destroy {
			ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
			_, _ = gw.DeleteVM(ctx2, &vmsanv1.DeleteVMRequest{VmId: vm.ID})
			cancel2()
			_ = store.Delete(vm.ID)
			hashStore := deploy.NewHashStore(p.BaseDir)
			_ = hashStore.Remove(vm.ID)
			return stopResult{Service: svcName, VmID: vm.ID, Status: "destroyed"}
		}
		return stopResult{Service: svcName, VmID: vm.ID, Status: "stopped"}
	}

	// Stop in reverse dependency order
	handled := make(map[string]bool)
	for _, svcName := range graph.ReverseOrder {
		vm := findProjectVM(allVMs, cfg.Project, svcName)
		if vm == nil {
			continue
		}
		results = append(results, stopVM(vm, svcName))
		handled[vm.ID] = true
	}

	// Handle remaining VMs not in the graph (accessories, etc.)
	for _, vm := range projectVMs {
		if handled[vm.ID] {
			continue
		}
		results = append(results, stopVM(vm, vm.Network.Service))
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	headers := []string{"SERVICE", "VM ID", "STATUS"}
	var rows [][]string
	for _, r := range results {
		status := r.Status
		switch r.Status {
		case "stopped":
			status = output.YellowText(status)
		case "destroyed":
			status = output.RedText(status)
		case "error":
			status = output.RedText(r.Status + ": " + r.Error)
		}
		rows = append(rows, []string{r.Service, r.VmID, status})
	}
	fmt.Println()
	output.PrintTable(os.Stdout, headers, rows)
	return nil
}
