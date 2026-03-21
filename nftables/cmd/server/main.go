package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/angelorc/vmsan/nftables/internal/server"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:6443", "listen address")
	dbPath := flag.String("db", "", "SQLite database path (default: ~/.vmsan/state.db)")
	flag.Parse()

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

	srv, err := server.New(*listen, *dbPath, logger)
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
