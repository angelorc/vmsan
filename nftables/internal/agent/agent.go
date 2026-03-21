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
	"path/filepath"
	"time"

	vmsync "github.com/angelorc/vmsan/nftables/internal/sync"
)

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

	handlers := vmsync.Handlers{
		OnVMCreate: func(payload json.RawMessage) error {
			logger.Info("vm create received", "payload", string(payload))
			// TODO: invoke Firecracker to start VM
			return nil
		},
		OnVMUpdate: func(payload json.RawMessage) error {
			logger.Info("vm update received", "payload", string(payload))
			// TODO: apply VM config changes
			return nil
		},
		OnVMDelete: func(id string) error {
			logger.Info("vm delete received", "id", id)
			// TODO: stop and clean up VM
			return nil
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
