package gateway

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type cfSetupParams struct {
	TunnelToken string `json:"tunnelToken"`
	ConfigPath  string `json:"configPath,omitempty"`
	LogPath     string `json:"logPath,omitempty"`
}

type cfAddRouteParams struct {
	VMId      string `json:"vmId"`
	Hostname  string `json:"hostname"`
	ApiToken  string `json:"apiToken"`
	TunnelId  string `json:"tunnelId"`
	AccountId string `json:"accountId"`
}

type cfRemoveRouteParams struct {
	VMId      string `json:"vmId"`
	ApiToken  string `json:"apiToken"`
	TunnelId  string `json:"tunnelId"`
	AccountId string `json:"accountId"`
}

// cloudflareState holds the running state of the cloudflared daemon.
var cfState struct {
	mu        sync.Mutex
	pid       int
	startTime time.Time
	cmd       *exec.Cmd
}

// cloudflaredBin returns the path to the cloudflared binary.
func cloudflaredBin() string {
	candidates := []string{
		"/usr/local/bin/cloudflared",
		"/usr/bin/cloudflared",
	}
	if entries, err := os.ReadDir("/home"); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				candidates = append(candidates, fmt.Sprintf("/home/%s/.vmsan/bin/cloudflared", e.Name()))
			}
		}
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "cloudflared"
}

// handleCloudflareSetup starts the cloudflared tunnel daemon.
func (s *Server) handleCloudflareSetup(params json.RawMessage) Response {
	var p cfSetupParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.TunnelToken == "" {
		return Response{OK: false, Error: "tunnelToken is required", Code: "VALIDATION_ERROR"}
	}

	cfState.mu.Lock()
	defer cfState.mu.Unlock()

	// Kill existing cloudflared if running.
	if cfState.pid > 0 {
		if proc, err := os.FindProcess(cfState.pid); err == nil {
			proc.Signal(syscall.SIGTERM)
			time.Sleep(2 * time.Second)
			proc.Kill()
		}
		cfState.pid = 0
		cfState.cmd = nil
	}

	bin := cloudflaredBin()
	args := []string{"tunnel", "--no-autoupdate", "run", "--token", p.TunnelToken}

	cmd := exec.Command(bin, args...)
	if p.LogPath != "" {
		logFile, err := os.OpenFile(p.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			slog.Warn("failed to open cloudflared log file", "path", p.LogPath, "error", err)
		} else {
			cmd.Stdout = logFile
			cmd.Stderr = logFile
		}
	}

	if err := cmd.Start(); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("start cloudflared: %s", err), Code: "CLOUDFLARE_ERROR"}
	}

	cfState.pid = cmd.Process.Pid
	cfState.startTime = time.Now()
	cfState.cmd = cmd

	// Reap process in background so it doesn't become a zombie.
	go func() {
		cmd.Wait()
		cfState.mu.Lock()
		if cfState.cmd == cmd {
			cfState.pid = 0
			cfState.cmd = nil
		}
		cfState.mu.Unlock()
	}()

	slog.Info("cloudflared started", "pid", cfState.pid)

	return Response{
		OK: true,
		VM: map[string]any{
			"pid": cfState.pid,
		},
	}
}

// handleCloudflareAddRoute is a stub — actual Cloudflare API calls stay in TS.
func (s *Server) handleCloudflareAddRoute(params json.RawMessage) Response {
	var p cfAddRouteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}
	if p.Hostname == "" {
		return Response{OK: false, Error: "hostname is required", Code: "VALIDATION_ERROR"}
	}
	// Stub — Cloudflare API calls remain in TypeScript.
	return Response{OK: true}
}

// handleCloudflareRemoveRoute is a stub — actual Cloudflare API calls stay in TS.
func (s *Server) handleCloudflareRemoveRoute(params json.RawMessage) Response {
	var p cfRemoveRouteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}
	// Stub — Cloudflare API calls remain in TypeScript.
	return Response{OK: true}
}

// handleCloudflareStatus checks if cloudflared is running.
func (s *Server) handleCloudflareStatus() Response {
	cfState.mu.Lock()
	defer cfState.mu.Unlock()

	if cfState.pid <= 0 {
		return Response{
			OK: true,
			VM: map[string]any{
				"running": false,
			},
		}
	}

	// Verify process is still alive.
	proc, err := os.FindProcess(cfState.pid)
	if err != nil {
		return Response{
			OK: true,
			VM: map[string]any{
				"running": false,
			},
		}
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		cfState.pid = 0
		cfState.cmd = nil
		return Response{
			OK: true,
			VM: map[string]any{
				"running": false,
			},
		}
	}

	uptime := time.Since(cfState.startTime).Truncate(time.Second).String()
	return Response{
		OK: true,
		VM: map[string]any{
			"running": true,
			"pid":     cfState.pid,
			"uptime":  uptime,
		},
	}
}
