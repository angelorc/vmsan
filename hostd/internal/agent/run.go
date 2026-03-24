package agent

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// RunAgentHost starts the agent-host daemon. This contains all the logic
// previously in cmd/vmsan-agent-host/main.go, supporting both the "join"
// subcommand and the default run mode. It parses from os.Args[2:] (skipping
// "vmsan" and "agent-host") and calls os.Exit on fatal errors.
func RunAgentHost() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	args := os.Args[2:]

	// Handle help flags
	for _, a := range args {
		if a == "--help" || a == "-h" || a == "-help" {
			fmt.Fprintf(os.Stderr, `Manage agent-host for multi-host clusters

Usage:
  vmsan agent-host [command]

Available Commands:
  join        Register this host with the control plane server
  run         Start the agent (default when no subcommand given)

Join Flags:
  --server    Server URL (e.g., http://10.88.0.1:6443)
  --token     Join token
  --name      Host name (defaults to OS hostname)
`)
			return
		}
	}

	if len(args) > 0 && args[0] == "join" {
		runJoin(logger, args[1:])
		return
	}

	// If subcommand is "run", skip it; otherwise just run with no args
	if len(args) > 0 && args[0] == "run" {
		args = args[1:]
	}

	runAgent(logger)
}

// runJoin handles the "join" subcommand: register with the server and
// persist configuration locally.
func runJoin(logger *slog.Logger, args []string) {
	joinCmd := flag.NewFlagSet("join", flag.ExitOnError)
	serverURL := joinCmd.String("server", "", "server URL (e.g., http://10.88.0.1:6443)")
	token := joinCmd.String("token", "", "join token")
	name := joinCmd.String("name", "", "host name (defaults to OS hostname)")

	if err := joinCmd.Parse(args); err != nil {
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

	if err := Join(*serverURL, *token, hostName, logger); err != nil {
		logger.Error("join failed", "error", err)
		os.Exit(1)
	}

	logger.Info("joined successfully")
}

// runAgent starts the normal agent operation mode: load persisted config,
// start sync engine and heartbeat loop, block until signal.
func runAgent(logger *slog.Logger) {
	a, err := New(logger)
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
