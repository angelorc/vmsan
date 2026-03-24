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

	// Configure binary locations. The gateway runs as root via systemd
	// and doesn't have the user's ~/.vmsan/bin in its PATH.
	fcDir := findBinary("firecracker", envOr("VMSAN_BIN_DIR", ""))

	if fcDir != "" {
		gateway.SetBinDir(fcDir)
		slog.Info("firecracker/jailer dir", "dir", fcDir)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	slots := gateway.NewSlotAllocator(254, "/run/vmsan/slots.json")
	meshManager := gateway.NewMeshManager(logger, slots)
	if err := meshManager.Start(ctx); err != nil {
		slog.Warn("mesh manager start failed, continuing without mesh", "error", err)
	}

	srv, err := gateway.NewServer(gateway.Config{
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

// findBinary locates the directory containing the named binary.
// Search order: explicit override, /usr/local/bin, /usr/bin, any /home/*/.vmsan/bin.
func findBinary(name, override string) string {
	if override != "" {
		if _, err := os.Stat(override + "/" + name); err == nil {
			return override
		}
	}
	for _, dir := range []string{"/usr/local/bin", "/usr/bin"} {
		if _, err := os.Stat(dir + "/" + name); err == nil {
			return dir
		}
	}
	// Scan /home/*/.vmsan/bin
	entries, _ := os.ReadDir("/home")
	for _, e := range entries {
		dir := fmt.Sprintf("/home/%s/.vmsan/bin", e.Name())
		if _, err := os.Stat(dir + "/" + name); err == nil {
			return dir
		}
	}
	return ""
}
