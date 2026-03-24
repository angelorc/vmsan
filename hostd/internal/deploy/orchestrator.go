package deploy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/config"
	"github.com/angelorc/vmsan/hostd/internal/gwclient"
)

// OrchestrateOptions are the inputs for orchestrating a full deployment.
type OrchestrateOptions struct {
	Config   *config.VmsanToml
	Project  string
	Gateway  *gwclient.Client
	Paths    DeployPaths
	Secrets  map[string]string // decrypted project secrets
	OnStatus func(service string, status DeployStatus)
}

// OrchestrateResult holds the outcome of a full multi-service deployment.
type OrchestrateResult struct {
	Services   []*ServiceDeployResult `json:"services"`
	Success    bool                   `json:"success"`
	DurationMs int64                  `json:"durationMs"`
}

// Orchestrate runs a full multi-service deployment:
// 1. Normalize to multi-service format
// 2. Build dependency graph
// 3. Deploy groups in topological order (parallel within each group)
// 4. Resolve reference variables after each group
func Orchestrate(ctx context.Context, opts OrchestrateOptions) (*OrchestrateResult, error) {
	start := time.Now()
	services := config.NormalizeToml(opts.Config)

	graph, err := BuildDependencyGraph(services, opts.Config.Accessories)
	if err != nil {
		return nil, fmt.Errorf("build dependency graph: %w", err)
	}

	var allResults []*ServiceDeployResult
	availableVars := make(map[string]string)

	// Merge secrets into available vars
	for k, v := range opts.Secrets {
		availableVars[k] = v
	}

	for _, group := range graph.Groups {
		var mu sync.Mutex
		var wg sync.WaitGroup
		groupFailed := false

		for _, svcName := range group.Services {
			svcCfg, ok := services[svcName]
			if !ok {
				// Could be an accessory — skip for now (accessories use pre-built images)
				continue
			}

			wg.Add(1)
			go func(name string, cfg config.ServiceConfig) {
				defer wg.Done()

				// Resolve env references
				env := config.ResolveReferences(cfg.Env, availableVars)

				// Merge secrets
				for k, v := range opts.Secrets {
					if _, exists := env[k]; !exists {
						env[k] = v
					}
				}

				result := DeployService(ctx, DeployServiceOptions{
					ServiceName: name,
					ServiceCfg:  cfg,
					Project:     opts.Project,
					Env:         env,
					Gateway:     opts.Gateway,
					Paths:       opts.Paths,
					OnStatus:    opts.OnStatus,
				})

				mu.Lock()
				allResults = append(allResults, result)
				if result.Status == StatusFailed {
					groupFailed = true
				}
				mu.Unlock()
			}(svcName, svcCfg)
		}

		wg.Wait()

		if groupFailed {
			// Stop deployment on group failure
			return &OrchestrateResult{
				Services:   allResults,
				Success:    false,
				DurationMs: time.Since(start).Milliseconds(),
			}, nil
		}

		// After each group, populate service variables for the next groups
		for _, result := range allResults {
			if result.Status == StatusRunning && result.VmID != "" {
				// Get mesh IP from result and generate variables
				svcCfg := services[result.Service]
				svcType := result.Service // default type is service name
				if _, ok := opts.Config.Accessories[result.Service]; ok {
					acc := opts.Config.Accessories[result.Service]
					svcType = acc.Type
				} else if svcCfg.Service != "" {
					svcType = svcCfg.Service
				}

				vars := config.GetServiceVariables(result.Service, svcType, "" /* meshIP from VM state */)
				for k, v := range vars {
					availableVars[result.Service+"."+k] = v
				}
			}
		}
	}

	return &OrchestrateResult{
		Services:   allResults,
		Success:    true,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}
