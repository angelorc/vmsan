package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/angelorc/vmsan/hostd/internal/gateway"
	"github.com/angelorc/vmsan/hostd/internal/netsetup"
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

	// Configure vmsan-nftables binary location. The gateway runs as root
	// and may not have the user's ~/.vmsan/bin in its PATH.
	nftBinDir := envOr("VMSAN_NFTABLES_BIN_DIR", "")
	if nftBinDir == "" {
		// Auto-detect from common install locations
		for _, dir := range []string{
			"/usr/local/bin",
			"/usr/bin",
		} {
			if _, err := os.Stat(dir + "/vmsan-nftables"); err == nil {
				nftBinDir = dir
				break
			}
		}
		// Check SUDO_USER's ~/.vmsan/bin (installer puts it there)
		if nftBinDir == "" {
			if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
				candidateDir := fmt.Sprintf("/home/%s/.vmsan/bin", sudoUser)
				if _, err := os.Stat(candidateDir + "/vmsan-nftables"); err == nil {
					nftBinDir = candidateDir
				}
			}
		}
		// Fallback: check all /home/*/.vmsan/bin
		if nftBinDir == "" {
			entries, _ := os.ReadDir("/home")
			for _, e := range entries {
				candidateDir := fmt.Sprintf("/home/%s/.vmsan/bin", e.Name())
				if _, err := os.Stat(candidateDir + "/vmsan-nftables"); err == nil {
					nftBinDir = candidateDir
					break
				}
			}
		}
	}
	if nftBinDir != "" {
		netsetup.SetNftablesBinDir(nftBinDir)
		slog.Info("nftables binary dir", "dir", nftBinDir)
	}

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
