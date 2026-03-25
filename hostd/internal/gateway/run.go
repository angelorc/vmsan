package gateway

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/angelorc/vmsan/hostd/internal/paths"
)

// RunGateway starts the gateway daemon. This contains all the logic previously
// in cmd/vmsan-gateway/main.go. It blocks until the context is cancelled
// (via SIGTERM/SIGINT) and calls os.Exit on fatal errors.
func RunGateway() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	socketPath := envOr("VMSAN_SOCKET", "/run/vmsan/gateway.sock")
	pidFile := envOr("VMSAN_PID_FILE", "/run/vmsan/gateway.pid")

	// Resolve paths using the same logic as the CLI.
	// This ensures the gateway and CLI agree on jailer dir, binary locations, etc.
	p := paths.Resolve()
	SetJailerBaseDir(p.JailerBaseDir)

	// Configure binary locations. The gateway runs as root via systemd
	// and doesn't have the user's ~/.vmsan/bin in its PATH.
	fcDir := findBinaryDir("firecracker", envOr("VMSAN_BIN_DIR", ""))

	if fcDir != "" {
		SetBinDir(fcDir)
		slog.Info("firecracker/jailer dir", "dir", fcDir)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	slots := NewSlotAllocator(254, "/run/vmsan/slots.json")
	meshManager := NewMeshManager(logger, slots)
	if err := meshManager.Start(ctx); err != nil {
		slog.Warn("mesh manager start failed, continuing without mesh", "error", err)
	}

	srv, err := NewServer(Config{
		SocketPath: socketPath,
		PIDFile:    pidFile,
	}, meshManager, slots)
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

// findBinaryDir locates the directory containing the named binary.
// Search order: explicit override, ~/.vmsan/bin, /usr/local/bin, /usr/bin, /home/*/.vmsan/bin.
func findBinaryDir(name, override string) string {
	if override != "" {
		if _, err := os.Stat(filepath.Join(override, name)); err == nil {
			return override
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, ".vmsan", "bin")
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return dir
		}
	}
	for _, dir := range []string{"/usr/local/bin", "/usr/bin"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return dir
		}
	}
	entries, _ := os.ReadDir("/home")
	for _, e := range entries {
		dir := filepath.Join("/home", e.Name(), ".vmsan", "bin")
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return dir
		}
	}
	return ""
}
