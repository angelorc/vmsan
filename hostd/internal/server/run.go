package server

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// RunServer starts the control plane server. This contains all the logic
// previously in cmd/vmsan-server/main.go. It parses flags from os.Args[2:]
// (skipping "vmsan" and "server") and calls os.Exit on fatal errors.
func RunServer() {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	listen := fs.String("listen", "0.0.0.0:6443", "listen address")
	dbPath := fs.String("db", "", "SQLite database path (default: ~/.vmsan/state.db)")
	if err := fs.Parse(os.Args[2:]); err != nil {
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Resolve default db path
	if *dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			logger.Error("failed to get home directory", "error", err)
			os.Exit(1)
		}
		*dbPath = home + "/.vmsan/state.db"
	}

	srv, err := New(*listen, *dbPath, logger)
	if err != nil {
		logger.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigCh
		logger.Info("shutting down")
		srv.Close()
	}()

	logger.Info("server starting", "addr", *listen, "db", *dbPath)
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
