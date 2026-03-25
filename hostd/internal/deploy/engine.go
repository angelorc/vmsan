package deploy

import (
	"context"
	"fmt"
	"os"
	"time"

	vmsanv1 "github.com/angelorc/vmsan/hostd/gen/vmsan/v1"
	"github.com/angelorc/vmsan/hostd/internal/agentclient"
	"github.com/angelorc/vmsan/hostd/internal/config"
	"github.com/angelorc/vmsan/hostd/internal/gwclient"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/vmstate"
	"github.com/angelorc/vmsan/hostd/internal/vmutil"
)

// DeployStatus tracks the current phase of a service deployment.
type DeployStatus string

const (
	StatusPending     DeployStatus = "pending"
	StatusCreating    DeployStatus = "creating"
	StatusUploading   DeployStatus = "uploading"
	StatusBuilding    DeployStatus = "building"
	StatusReleasing   DeployStatus = "releasing"
	StatusStarting    DeployStatus = "starting"
	StatusHealthCheck DeployStatus = "health_check"
	StatusRunning     DeployStatus = "running"
	StatusFailed      DeployStatus = "failed"
	StatusSkipped     DeployStatus = "skipped"
)

// ServiceDeployResult holds the outcome of deploying a single service.
type ServiceDeployResult struct {
	Service    string       `json:"service"`
	Status     DeployStatus `json:"status"`
	VmID       string       `json:"vmId,omitempty"`
	Error      string       `json:"error,omitempty"`
	Skipped    bool         `json:"skipped,omitempty"`
	DurationMs int64        `json:"durationMs"`
}

// DeployServiceOptions are the inputs for deploying a single service.
type DeployServiceOptions struct {
	ServiceName string
	ServiceCfg  config.ServiceConfig
	Project     string
	Env         map[string]string // resolved env vars (including references + secrets)
	Gateway     *gwclient.Client
	Paths       DeployPaths
	OnStatus    func(service string, status DeployStatus)
}

// DeployPaths holds paths needed during deployment.
type DeployPaths struct {
	BaseDir   string
	SourceDir string // local project directory to upload
	AgentPort int
}

// DeployService handles the full lifecycle of deploying a single service VM:
// create -> wait for agent -> upload -> build -> release -> start -> health check
func DeployService(ctx context.Context, opts DeployServiceOptions) *ServiceDeployResult {
	start := time.Now()
	result := &ServiceDeployResult{
		Service: opts.ServiceName,
		Status:  StatusPending,
	}

	setStatus := func(s DeployStatus) {
		result.Status = s
		if opts.OnStatus != nil {
			opts.OnStatus(opts.ServiceName, s)
		}
	}

	fail := func(err error) *ServiceDeployResult {
		result.Status = StatusFailed
		result.Error = err.Error()
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	// Check for existing running VM — reuse it instead of creating a new one
	p := paths.Resolve()
	store := vmstate.NewStore(p.VmsDir)
	var vmID, guestIP, token string

	existingVMs, _ := store.List()
	for _, vm := range existingVMs {
		if vm.Project == opts.Project && vm.Network.Service == opts.ServiceName && vm.Status == "running" {
			vmID = vm.ID
			guestIP = vm.Network.GuestIP
			if vm.AgentToken != nil {
				token = *vm.AgentToken
			}
			break
		}
	}

	if vmID == "" {
		// Phase 1: Create VM
		setStatus(StatusCreating)
		var err error
		vmID, guestIP, token, err = createServiceVM(ctx, opts)
		if err != nil {
			return fail(fmt.Errorf("create VM: %w", err))
		}
	}
	result.VmID = vmID

	// Phase 2: Wait for agent
	agentURL := fmt.Sprintf("http://%s:%d", guestIP, opts.Paths.AgentPort)
	agent := agentclient.New(agentURL, token)
	if err := waitForAgentReady(ctx, agent); err != nil {
		return fail(fmt.Errorf("agent not ready: %w", err))
	}

	// Phase 3: Upload source
	if opts.Paths.SourceDir != "" {
		setStatus(StatusUploading)
		if err := UploadSource(ctx, agent, opts.Paths.SourceDir); err != nil {
			return fail(fmt.Errorf("upload: %w", err))
		}
	}

	// Phase 4: Build
	if opts.ServiceCfg.Build != "" {
		setStatus(StatusBuilding)
		buildResult, err := ExecuteBuild(ctx, agent, opts.ServiceCfg.Build, opts.Env)
		if err != nil {
			return fail(fmt.Errorf("build: %w", err))
		}
		if !buildResult.Success {
			return fail(fmt.Errorf("build failed (exit %d): %s", buildResult.ExitCode, buildResult.Output))
		}
	}

	// Phase 5: Release
	if opts.ServiceCfg.Start != "" {
		release := ""
		if opts.Env != nil {
			// Check for deploy.release in parent config
		}
		if release != "" {
			setStatus(StatusReleasing)
			releaseResult, err := ExecuteRelease(ctx, agent, release, opts.Env)
			if err != nil {
				return fail(fmt.Errorf("release: %w", err))
			}
			if !releaseResult.Success {
				return fail(fmt.Errorf("release failed (exit %d): %s", releaseResult.ExitCode, releaseResult.Output))
			}
		}
	}

	// Phase 6: Start
	if opts.ServiceCfg.Start != "" {
		setStatus(StatusStarting)
		if err := StartApp(ctx, agent, opts.ServiceCfg.Start, opts.Env); err != nil {
			return fail(fmt.Errorf("start: %w", err))
		}
	}

	// Phase 7: Health check
	if opts.ServiceCfg.HealthCheck != nil {
		setStatus(StatusHealthCheck)
		if err := runHealthCheck(ctx, agent, opts.ServiceCfg.HealthCheck); err != nil {
			return fail(fmt.Errorf("health check: %w", err))
		}
	}

	setStatus(StatusRunning)
	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

func createServiceVM(ctx context.Context, opts DeployServiceOptions) (vmID, guestIP, agentToken string, err error) {
	cfg := opts.ServiceCfg
	p := paths.Resolve()

	// Resolve runtime, defaulting to "base"
	runtime := cfg.Runtime
	if runtime == "" {
		runtime = "base"
	}

	// Auto-detect kernel and rootfs
	kernelPath := vmutil.AutoDetectKernel(p.KernelsDir)
	rootfsPath := vmutil.AutoDetectRootfs(p.RootfsDir, runtime)

	// Generate agent token
	agentToken = vmutil.GenerateToken()

	// Resolve disk size (default 10 GB)
	var diskSizeGb float64 = 10
	if cfg.Disk != "" {
		parsed, err := vmutil.ParseDiskSize(cfg.Disk)
		if err != nil {
			return "", "", "", fmt.Errorf("parse disk size: %w", err)
		}
		diskSizeGb = float64(parsed)
	}

	// Resolve vcpus and memory with defaults
	vcpus := int32(1)
	if cfg.Vcpus > 0 {
		vcpus = int32(cfg.Vcpus)
	}
	memMib := int32(128)
	if cfg.Memory > 0 {
		memMib = int32(cfg.Memory)
	}

	// Network policy
	networkPolicy := cfg.NetworkPolicy
	if networkPolicy == "" {
		networkPolicy = "allow-all"
	}
	// Auto-promote to custom when domains are provided
	if networkPolicy == "allow-all" && len(cfg.AllowedDomains) > 0 {
		networkPolicy = "custom"
	}

	// Convert published ports to int32
	ports := make([]int32, len(cfg.PublishPorts))
	for i, port := range cfg.PublishPorts {
		ports[i] = int32(port)
	}

	// Build gRPC request
	req := &vmsanv1.CreateVMRequest{
		Vcpus:         vcpus,
		MemMib:        memMib,
		Runtime:       runtime,
		DiskSizeGb:    diskSizeGb,
		NetworkPolicy: networkPolicy,
		Domains:       cfg.AllowedDomains,
		Ports:         ports,
		Project:       opts.Project,
		Service:       opts.ServiceName,
		ConnectTo:     cfg.ConnectTo,
		KernelPath:    kernelPath,
		RootfsPath:    rootfsPath,
		AgentBinary:   p.AgentBin,
		AgentToken:    agentToken,
		SeccompFilter: p.SeccompFilter,
		JailerBaseDir: p.JailerBaseDir,
		OwnerUid:      int32(os.Getuid()),
		OwnerGid:      int32(os.Getgid()),
	}

	resp, err := opts.Gateway.CreateVM(ctx, req)
	if err != nil {
		return "", "", "", fmt.Errorf("gateway CreateVM: %w", err)
	}

	// Build and save local state
	pid := int(resp.Pid)
	state := &vmstate.VmState{
		ID:         resp.VmId,
		Project:    opts.Project,
		Runtime:    runtime,
		DiskSizeGb: diskSizeGb,
		Status:     "running",
		PID:        &pid,
		APISocket:  resp.SocketPath,
		ChrootDir:  resp.ChrootDir,
		Kernel:     kernelPath,
		Rootfs:     rootfsPath,
		VcpuCount:  int(vcpus),
		MemSizeMib: int(memMib),
		Network: vmstate.VmNetwork{
			TapDevice:      resp.TapDevice,
			HostIP:         resp.HostIp,
			GuestIP:        resp.GuestIp,
			SubnetMask:     resp.SubnetMask,
			MACAddress:     resp.MacAddress,
			NetworkPolicy:  networkPolicy,
			AllowedDomains: cfg.AllowedDomains,
			PublishedPorts: cfg.PublishPorts,
			NetNSName:      resp.NetNsName,
			AllowIcmp:      false,
			FirewallBackend: "nftables",
			ConnectTo:      cfg.ConnectTo,
			MeshIP:         resp.MeshIp,
			Service:        opts.ServiceName,
		},
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		AgentToken:   &agentToken,
		AgentPort:    p.AgentPort,
		StateVersion: 2,
	}

	store := vmstate.NewStore(p.VmsDir)
	if err := store.Save(state); err != nil {
		return "", "", "", fmt.Errorf("save state: %w", err)
	}

	return resp.VmId, resp.GuestIp, agentToken, nil
}

func waitForAgentReady(ctx context.Context, agent *agentclient.Client) error {
	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("agent did not become ready within 30s")
		case <-ticker.C:
			if err := agent.Health(ctx); err == nil {
				return nil
			}
		}
	}
}

func runHealthCheck(ctx context.Context, agent *agentclient.Client, hc *config.HealthCheckConfig) error {
	timeout := 90 * time.Second
	if hc.Timeout > 0 {
		timeout = time.Duration(hc.Timeout) * time.Second
	}
	interval := 2 * time.Second
	if hc.Interval > 0 {
		interval = time.Duration(hc.Interval) * time.Second
	}
	retries := 30
	if hc.Retries > 0 {
		retries = hc.Retries
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	attempts := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("health check timed out after %s", timeout)
		case <-ticker.C:
			attempts++
			if err := agent.Health(ctx); err == nil {
				return nil
			}
			if attempts >= retries {
				return fmt.Errorf("health check failed after %d retries", retries)
			}
		}
	}
}
