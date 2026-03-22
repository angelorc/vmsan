package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/angelorc/vmsan/hostd/internal/gateway"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Accept "start" subcommand or no args (for systemd ExecStart)
	if len(os.Args) > 1 && os.Args[1] != "start" {
		fmt.Fprintf(os.Stderr, "usage: vmsan-gateway [start]\n")
		os.Exit(1)
	}

	socketPath := envOr("VMSAN_SOCKET", "/run/vmsan/gateway.sock")
	pidFile := envOr("VMSAN_PID_FILE", "/run/vmsan/gateway.pid")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	meshManager := gateway.NewMeshManager(logger)
	if err := meshManager.Start(ctx); err != nil {
		slog.Warn("mesh manager start failed, continuing without mesh", "error", err)
	}

	srv, err := gateway.NewServer(gateway.Config{
		SocketPath: socketPath,
		PIDFile:    pidFile,
	}, meshManager)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	if err := srv.Run(ctx); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}

	if err := meshManager.Stop(); err != nil {
		slog.Debug("mesh manager stop error", "error", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
