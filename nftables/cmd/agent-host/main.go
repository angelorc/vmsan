// Command agent-host is the vmsan agent worker that runs on remote hosts.
// It registers with the control plane server, polls for VM lifecycle
// changes, and reports host health via periodic heartbeats.
//
// Usage:
//
//	vmsan-agent-host join --server <url> --token <token> [--name <hostname>]
//	vmsan-agent-host
package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/angelorc/vmsan/nftables/internal/agent"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	if len(os.Args) > 1 && os.Args[1] == "join" {
		runJoin(logger)
		return
	}

	runAgent(logger)
}

// runJoin handles the "join" subcommand: register with the server and
// persist configuration locally.
func runJoin(logger *slog.Logger) {
	joinCmd := flag.NewFlagSet("join", flag.ExitOnError)
	serverURL := joinCmd.String("server", "", "server URL (e.g., http://10.88.0.1:6443)")
	token := joinCmd.String("token", "", "join token")
	name := joinCmd.String("name", "", "host name (defaults to OS hostname)")

	if err := joinCmd.Parse(os.Args[2:]); err != nil {
		os.Exit(1)
	}

	if *serverURL == "" || *token == "" {
		logger.Error("--server and --token are required")
		os.Exit(1)
	}

	hostName := *name
	if hostName == "" {
		var err error
		hostName, err = os.Hostname()
		if err != nil {
			logger.Error("failed to get hostname", "error", err)
			os.Exit(1)
		}
	}

	if err := agent.Join(*serverURL, *token, hostName, logger); err != nil {
		logger.Error("join failed", "error", err)
		os.Exit(1)
	}

	logger.Info("joined successfully")
}

// runAgent starts the normal agent operation mode: load persisted config,
// start sync engine and heartbeat loop, block until signal.
func runAgent(logger *slog.Logger) {
	a, err := agent.New(logger)
	if err != nil {
		logger.Error("failed to create agent", "error", err)
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig)
		a.Stop()
	}()

	logger.Info("agent starting")
	if err := a.Run(); err != nil {
		logger.Error("agent error", "error", err)
		os.Exit(1)
	}
	logger.Info("agent stopped")
}
