package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/angelorc/vmsan/nftables/internal/gateway"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if len(os.Args) < 2 || os.Args[1] != "start" {
		fmt.Fprintf(os.Stderr, "usage: vmsan-gateway start\n")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	srv, err := gateway.NewServer(gateway.Config{
		SocketPath: "/run/vmsan-gateway.sock",
		PIDFile:    "/run/vmsan-gateway.pid",
	})
	if err != nil {
		slog.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	if err := srv.Run(ctx); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
