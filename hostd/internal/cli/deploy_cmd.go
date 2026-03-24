package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/agentclient"
	"github.com/angelorc/vmsan/hostd/internal/config"
	"github.com/angelorc/vmsan/hostd/internal/deploy"
	"github.com/angelorc/vmsan/hostd/internal/output"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/secrets"
	"github.com/angelorc/vmsan/hostd/internal/vmstate"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy [service]",
	Short: "Re-deploy services (upload, build, start) without recreating VMs",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDeploy,
}

func init() {
	deployCmd.Flags().String("config", "vmsan.toml", "Path to vmsan.toml")
	rootCmd.AddCommand(deployCmd)
}

func runDeploy(cmd *cobra.Command, args []string) error {
	cfgPath, _ := cmd.Flags().GetString("config")
	targetService := ""
	if len(args) > 0 {
		targetService = args[0]
	}

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
	graph, err := deploy.BuildDependencyGraph(services, cfg.Accessories)
	if err != nil {
		return fmt.Errorf("build dependency graph: %w", err)
	}

	// Load secrets
	secretStore := secrets.NewStore(p.BaseDir)
	projectSecrets, _ := secretStore.GetAll(cfg.Project)

	cwd, _ := os.Getwd()

	type deployResult struct {
		Service    string `json:"service"`
		Status     string `json:"status"`
		Error      string `json:"error,omitempty"`
		DurationMs int64  `json:"durationMs"`
	}
	var results []deployResult

	// Deploy in dependency order
	for _, svcName := range graph.Order {
		if targetService != "" && svcName != targetService {
			continue
		}

		svcCfg, ok := services[svcName]
		if !ok {
			continue
		}

		// Find existing VM for this service
		vm := findProjectVM(allVMs, cfg.Project, svcName)
		if vm == nil {
			results = append(results, deployResult{
				Service: svcName,
				Status:  "skipped",
				Error:   "no running VM found",
			})
			continue
		}

		start := time.Now()

		token := ""
		if vm.AgentToken != nil {
			token = *vm.AgentToken
		}
		agent := agentclient.New(
			fmt.Sprintf("http://%s:%d", vm.Network.GuestIP, vm.AgentPort),
			token,
		)

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

		// Resolve env
		env := config.ResolveReferences(svcCfg.Env, nil)
		for k, v := range projectSecrets {
			if _, exists := env[k]; !exists {
				env[k] = v
			}
		}

		// Stop running app process
		stopRunningApp(ctx, agent)

		// Upload source
		if err := deploy.UploadSource(ctx, agent, cwd); err != nil {
			cancel()
			results = append(results, deployResult{
				Service:    svcName,
				Status:     "failed",
				Error:      fmt.Sprintf("upload: %s", err),
				DurationMs: time.Since(start).Milliseconds(),
			})
			continue
		}

		// Build
		if svcCfg.Build != "" {
			buildResult, err := deploy.ExecuteBuild(ctx, agent, svcCfg.Build, env)
			if err != nil || !buildResult.Success {
				cancel()
				errMsg := "build failed"
				if err != nil {
					errMsg = err.Error()
				} else {
					errMsg = fmt.Sprintf("exit %d: %s", buildResult.ExitCode, truncate(buildResult.Output, 200))
				}
				results = append(results, deployResult{
					Service:    svcName,
					Status:     "failed",
					Error:      errMsg,
					DurationMs: time.Since(start).Milliseconds(),
				})
				continue
			}
		}

		// Release
		if cfg.Deploy.Release != "" {
			releaseResult, err := deploy.ExecuteRelease(ctx, agent, cfg.Deploy.Release, env)
			if err != nil || !releaseResult.Success {
				cancel()
				errMsg := "release failed"
				if err != nil {
					errMsg = err.Error()
				}
				results = append(results, deployResult{
					Service:    svcName,
					Status:     "failed",
					Error:      errMsg,
					DurationMs: time.Since(start).Milliseconds(),
				})
				continue
			}
		}

		// Start
		if svcCfg.Start != "" {
			if err := deploy.StartApp(ctx, agent, svcCfg.Start, env); err != nil {
				cancel()
				results = append(results, deployResult{
					Service:    svcName,
					Status:     "failed",
					Error:      fmt.Sprintf("start: %s", err),
					DurationMs: time.Since(start).Milliseconds(),
				})
				continue
			}
		}

		cancel()
		results = append(results, deployResult{
			Service:    svcName,
			Status:     "deployed",
			DurationMs: time.Since(start).Milliseconds(),
		})
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	headers := []string{"SERVICE", "STATUS", "DURATION"}
	var rows [][]string
	for _, r := range results {
		status := r.Status
		if r.Status == "deployed" {
			status = output.GreenText(status)
		} else if r.Status == "failed" {
			status = output.RedText(status)
		}
		dur := fmt.Sprintf("%.1fs", float64(r.DurationMs)/1000)
		rows = append(rows, []string{r.Service, status, dur})
	}
	fmt.Println()
	output.PrintTable(os.Stdout, headers, rows)

	for _, r := range results {
		if r.Error != "" {
			fmt.Printf("  %s: %s\n", output.RedText(r.Service), r.Error)
		}
	}

	return nil
}

func findProjectVM(vms []*vmstate.VmState, project, service string) *vmstate.VmState {
	for _, vm := range vms {
		if vm.Project == project && vm.Network.Service == service && vm.Status == "running" {
			return vm
		}
	}
	return nil
}

func stopRunningApp(ctx context.Context, agent *agentclient.Client) {
	// Kill any running app process. Best-effort — ignore errors.
	_, _ = agent.Exec(ctx, agentclient.RunParams{
		Cmd:  "sh",
		Args: []string{"-c", "pkill -f 'nohup' 2>/dev/null; sleep 1"},
	})
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
