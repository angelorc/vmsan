package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/agentclient"
	"github.com/angelorc/vmsan/hostd/internal/config"
	"github.com/angelorc/vmsan/hostd/internal/output"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/vmstate"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project service status overview",
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().String("config", "vmsan.toml", "Path to vmsan.toml")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, _ []string) error {
	cfgPath, _ := cmd.Flags().GetString("config")

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

	services := config.NormalizeToml(cfg)

	type serviceStatus struct {
		Service  string `json:"service"`
		VmID     string `json:"vmId,omitempty"`
		Status   string `json:"status"`
		Health   string `json:"health,omitempty"`
		Memory   string `json:"memory,omitempty"`
		Endpoint string `json:"endpoint,omitempty"`
	}
	var statuses []serviceStatus

	for svcName := range services {
		vm := findProjectVM(allVMs, cfg.Project, svcName)
		ss := serviceStatus{Service: svcName}

		if vm == nil {
			ss.Status = "not deployed"
			statuses = append(statuses, ss)
			continue
		}

		ss.VmID = vm.ID
		ss.Status = vm.Status
		ss.Memory = fmt.Sprintf("%d MiB", vm.MemSizeMib)
		ss.Endpoint = fmt.Sprintf("%s.%s.vmsan.internal", svcName, cfg.Project)

		// Health check (2s timeout)
		if vm.Status == "running" {
			ss.Health = checkServiceHealth(vm)
		}

		statuses = append(statuses, ss)
	}

	// Also show accessories
	for accName := range cfg.Accessories {
		vm := findProjectVM(allVMs, cfg.Project, accName)
		ss := serviceStatus{Service: accName}

		if vm == nil {
			ss.Status = "not deployed"
		} else {
			ss.VmID = vm.ID
			ss.Status = vm.Status
			ss.Memory = fmt.Sprintf("%d MiB", vm.MemSizeMib)
			if vm.Status == "running" {
				ss.Health = checkServiceHealth(vm)
			}
		}

		statuses = append(statuses, ss)
	}

	if jsonOutput {
		data, _ := json.Marshal(map[string]any{"services": statuses})
		fmt.Println(string(data))
		return nil
	}

	headers := []string{"SERVICE", "VM ID", "STATUS", "HEALTH", "MEMORY", "ENDPOINT"}
	var rows [][]string
	for _, ss := range statuses {
		vmID := ss.VmID
		if vmID == "" {
			vmID = "-"
		}
		status := output.StatusColor(ss.Status)
		health := ss.Health
		if health == "" {
			health = "-"
		} else if health == "healthy" {
			health = output.GreenText(health)
		} else {
			health = output.RedText(health)
		}
		memory := ss.Memory
		if memory == "" {
			memory = "-"
		}
		endpoint := ss.Endpoint
		if endpoint == "" {
			endpoint = "-"
		}
		rows = append(rows, []string{ss.Service, vmID, status, health, memory, endpoint})
	}
	fmt.Println()
	output.PrintTable(os.Stdout, headers, rows)
	return nil
}

func checkServiceHealth(vm *vmstate.VmState) string {
	token := ""
	if vm.AgentToken != nil {
		token = *vm.AgentToken
	}
	agent := agentclient.New(
		fmt.Sprintf("http://%s:%d", vm.Network.GuestIP, vm.AgentPort),
		token,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := agent.Health(ctx); err != nil {
		return "unhealthy"
	}
	return "healthy"
}
