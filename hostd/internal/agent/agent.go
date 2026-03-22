// Package agent implements the vmsan agent-host worker that runs on remote
// hosts, connects to the control plane server, reports health and resources,
// and processes VM lifecycle commands via the pull-based sync engine.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	vmsync "github.com/angelorc/vmsan/hostd/internal/sync"
)

// gatewaySocketPath is the default Unix socket path for the local gateway.
const gatewaySocketPath = "/run/vmsan/gateway.sock"

// Config holds the persisted agent configuration, written during join and
// loaded on subsequent starts.
type Config struct {
	ServerURL string `json:"server_url"`
	HostID    string `json:"host_id"`
	HostName  string `json:"host_name"`
}

// Agent is the main agent-host worker.
type Agent struct {
	config *Config
	logger *slog.Logger
	http   *http.Client
	sync   *vmsync.Engine
	cancel context.CancelFunc
}

// configPath returns the path to the agent config file.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".vmsan", "agent.json"), nil
}

// loadConfig reads the agent config from ~/.vmsan/agent.json.
func loadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.ServerURL == "" || cfg.HostID == "" {
		return nil, fmt.Errorf("config is incomplete: server_url and host_id are required")
	}

	return &cfg, nil
}

// saveConfig writes the agent config to ~/.vmsan/agent.json.
func saveConfig(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// New creates a new agent by loading persisted configuration and
// initialising the sync engine. Call Run to start processing.
func New(logger *slog.Logger) (*Agent, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("load agent config: %w", err)
	}

	gw := NewGatewayClient(gatewaySocketPath)

	handlers := vmsync.Handlers{
		OnVMCreate: func(payload json.RawMessage) error {
			logger.Info("vm create received", "payload_size", len(payload))

			var params VMCreateParams
			if err := json.Unmarshal(payload, &params); err != nil {
				return fmt.Errorf("unmarshal vm create params: %w", err)
			}

			result, err := gw.VMCreate(params)
			if err != nil {
				return fmt.Errorf("gateway vm.create: %w", err)
			}

			logger.Info("vm created via gateway",
				"vmId", result.VMId,
				"pid", result.PID,
				"guestIp", result.GuestIP,
			)
			return nil
		},

		OnVMUpdate: func(payload json.RawMessage) error {
			logger.Info("vm update received")

			var params struct {
				VMId   string `json:"vmId"`
				Policy string `json:"policy"`
			}
			if err := json.Unmarshal(payload, &params); err != nil {
				return fmt.Errorf("unmarshal vm update: %w", err)
			}

			return gw.VMUpdatePolicy(params.VMId, params.Policy)
		},

		OnVMDelete: func(id string) error {
			logger.Info("vm delete received", "id", id)
			return gw.VMDelete(id)
		},
	}

	syncEngine := vmsync.New(cfg.ServerURL, cfg.HostID, handlers, logger)

	return &Agent{
		config: cfg,
		logger: logger,
		http:   &http.Client{Timeout: 30 * time.Second},
		sync:   syncEngine,
	}, nil
}

// Run starts the agent's sync and heartbeat loops. It blocks until
// Stop is called or an unrecoverable error occurs.
func (a *Agent) Run() error {
	// Ensure the local gateway is running before processing sync commands.
	if err := a.ensureGateway(); err != nil {
		return fmt.Errorf("ensure gateway: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	a.logger.Info("agent running",
		"host_id", a.config.HostID,
		"host_name", a.config.HostName,
		"server", a.config.ServerURL,
	)

	// Start heartbeat in background.
	go a.heartbeatLoop(ctx)

	// Run sync engine (blocks until ctx cancelled).
	return a.sync.Run(ctx)
}

// Stop cancels the agent's context, triggering graceful shutdown of the
// sync engine and heartbeat loop.
func (a *Agent) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
}

// ensureGateway checks that the local gateway is running.
// If not, tries to start it via systemctl.
func (a *Agent) ensureGateway() error {
	gw := NewGatewayClient(gatewaySocketPath)
	if err := gw.Ping(); err == nil {
		return nil // already running
	}

	// Try systemctl
	a.logger.Info("gateway not running, attempting to start via systemctl")
	cmd := exec.Command("systemctl", "start", "vmsan-gateway")
	if err := cmd.Run(); err != nil {
		a.logger.Warn("systemctl start vmsan-gateway failed", "error", err)
		return fmt.Errorf("gateway not running and could not start: %w", err)
	}

	// Wait for socket
	for i := 0; i < 25; i++ {
		time.Sleep(200 * time.Millisecond)
		if err := gw.Ping(); err == nil {
			a.logger.Info("gateway started successfully")
			return nil
		}
	}

	return fmt.Errorf("gateway did not become ready after 5 seconds")
}
