package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/config"
	"github.com/angelorc/vmsan/hostd/internal/deploy"
	"github.com/angelorc/vmsan/hostd/internal/gwclient"
	"github.com/angelorc/vmsan/hostd/internal/output"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/secrets"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Deploy all services from vmsan.toml",
	RunE:  runUp,
}

func init() {
	upCmd.Flags().String("config", "vmsan.toml", "Path to vmsan.toml")
	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, _ []string) error {
	cfgPath, _ := cmd.Flags().GetString("config")

	cfg, err := config.LoadVmsanToml(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Validate
	if errs := config.ValidateToml(cfg); len(errs) > 0 {
		fmt.Printf("Config validation failed with %d error(s). Run 'vmsan validate' for details.\n", len(errs))
		os.Exit(1)
	}

	p := paths.Resolve()
	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer gw.Close()

	// Load secrets
	store := secrets.NewStore(p.BaseDir)
	projectSecrets, _ := store.GetAll(cfg.Project)

	cwd, _ := os.Getwd()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	result, err := deploy.Orchestrate(ctx, deploy.OrchestrateOptions{
		Config:  cfg,
		Project: cfg.Project,
		Gateway: gw,
		Paths: deploy.DeployPaths{
			BaseDir:   p.BaseDir,
			SourceDir: cwd,
			AgentPort: p.AgentPort,
		},
		Secrets: projectSecrets,
		OnStatus: func(service string, status deploy.DeployStatus) {
			if verbose {
				fmt.Printf("  [%s] %s\n", service, status)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("deploy: %w", err)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Print results table
	headers := []string{"SERVICE", "STATUS", "VM ID", "DURATION"}
	var rows [][]string
	for _, svc := range result.Services {
		status := string(svc.Status)
		if svc.Status == deploy.StatusRunning {
			status = output.GreenText(status)
		} else if svc.Status == deploy.StatusFailed {
			status = output.RedText(status)
		}

		dur := fmt.Sprintf("%.1fs", float64(svc.DurationMs)/1000)
		vmID := svc.VmID
		if vmID == "" {
			vmID = "-"
		}
		rows = append(rows, []string{svc.Service, status, vmID, dur})
	}
	fmt.Println()
	output.PrintTable(os.Stdout, headers, rows)

	if !result.Success {
		fmt.Println()
		for _, svc := range result.Services {
			if svc.Error != "" {
				fmt.Printf("  %s: %s\n", output.RedText(svc.Service), svc.Error)
			}
		}
		os.Exit(1)
	}

	fmt.Printf("\n  Deployed in %.1fs\n", float64(result.DurationMs)/1000)
	return nil
}
