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

	// Configure binary locations. The gateway runs as root via systemd
	// and doesn't have the user's ~/.vmsan/bin in its PATH.
	binDir := findBinDir(envOr("VMSAN_BIN_DIR", ""))
	if binDir != "" {
		netsetup.SetNftablesBinDir(binDir)
		gateway.SetBinDir(binDir)
		slog.Info("binary dir", "dir", binDir)
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

// findBinDir locates the directory containing vmsan binaries (firecracker,
// jailer, vmsan-nftables). It checks: explicit override, /usr/local/bin,
// /usr/bin, SUDO_USER's ~/.vmsan/bin, any user's ~/.vmsan/bin.
func findBinDir(override string) string {
	if override != "" {
		return override
	}
	// Check standard system paths
	for _, dir := range []string{"/usr/local/bin", "/usr/bin"} {
		if _, err := os.Stat(dir + "/firecracker"); err == nil {
			return dir
		}
	}
	// Check SUDO_USER's ~/.vmsan/bin
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		dir := fmt.Sprintf("/home/%s/.vmsan/bin", sudoUser)
		if _, err := os.Stat(dir + "/firecracker"); err == nil {
			return dir
		}
	}
	// Scan /home/*/.vmsan/bin
	entries, _ := os.ReadDir("/home")
	for _, e := range entries {
		dir := fmt.Sprintf("/home/%s/.vmsan/bin", e.Name())
		if _, err := os.Stat(dir + "/firecracker"); err == nil {
			return dir
		}
	}
	return ""
}
