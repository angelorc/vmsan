package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"syscall"
	"time"

	types "github.com/angelorc/vmsan/nftables"
	"github.com/angelorc/vmsan/nftables/internal/firewall"
)

const (
	stdinTimeout = 5 * time.Second
	cmdTimeout   = 30 * time.Second
)

// Exit codes per the vmsan-nftables interface contract.
const (
	exitSuccess    = 0
	exitNftError   = 1
	exitParseError = 2
	exitPermDenied = 3
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if len(os.Args) < 2 {
		writeResult(&types.NftResult{OK: false, Error: "usage: vmsan-nftables <setup|teardown|verify>", Code: "MISSING_COMMAND"})
		return exitParseError
	}

	input, err := readStdin()
	if err != nil {
		writeResult(&types.NftResult{OK: false, Error: fmt.Sprintf("read stdin: %v", err), Code: "STDIN_ERROR"})
		return exitParseError
	}

	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	switch os.Args[1] {
	case "setup":
		return handleSetup(ctx, input)
	case "teardown":
		return handleTeardown(ctx, input)
	case "verify":
		return handleVerify(ctx, input)
	default:
		writeResult(&types.NftResult{OK: false, Error: fmt.Sprintf("unknown command: %s", os.Args[1]), Code: "UNKNOWN_COMMAND"})
		return exitParseError
	}
}

func handleSetup(ctx context.Context, input []byte) int {
	var cfg types.SetupConfig
	if err := json.Unmarshal(input, &cfg); err != nil {
		return writeParseError(fmt.Errorf("parse config: %w", err))
	}
	if err := cfg.Validate(); err != nil {
		return writeValidationError(err)
	}

	opts := cfg.ToOptions()
	slog.InfoContext(ctx, "starting setup", "vm_id", opts.VMId)
	if err := firewall.Setup(ctx, opts); err != nil {
		slog.ErrorContext(ctx, "setup failed", "vm_id", opts.VMId, "error", err)
		return writeNftError(err)
	}
	slog.InfoContext(ctx, "setup complete", "vm_id", opts.VMId)
	if err := writeResult(&types.NftResult{OK: true}); err != nil {
		return writeNftError(err)
	}
	return exitSuccess
}

func handleTeardown(ctx context.Context, input []byte) int {
	var cfg types.TeardownConfig
	if err := json.Unmarshal(input, &cfg); err != nil {
		return writeParseError(fmt.Errorf("parse config: %w", err))
	}
	if err := cfg.Validate(); err != nil {
		return writeValidationError(err)
	}

	opts := cfg.ToOptions()
	slog.InfoContext(ctx, "starting teardown", "vm_id", opts.VMId)
	if err := firewall.Teardown(ctx, opts); err != nil {
		slog.ErrorContext(ctx, "teardown failed", "vm_id", opts.VMId, "error", err)
		return writeNftError(err)
	}
	slog.InfoContext(ctx, "teardown complete", "vm_id", opts.VMId)
	if err := writeResult(&types.NftResult{OK: true}); err != nil {
		return writeNftError(err)
	}
	return exitSuccess
}

func handleVerify(ctx context.Context, input []byte) int {
	var cfg types.VerifyConfig
	if err := json.Unmarshal(input, &cfg); err != nil {
		return writeParseError(fmt.Errorf("parse config: %w", err))
	}
	if err := cfg.Validate(); err != nil {
		return writeValidationError(err)
	}

	opts := cfg.ToOptions()
	slog.DebugContext(ctx, "starting verify", "vm_id", opts.VMId)
	result, err := firewall.Verify(ctx, opts)
	if err != nil {
		slog.ErrorContext(ctx, "verify failed", "vm_id", opts.VMId, "error", err)
		return writeNftError(err)
	}
	slog.DebugContext(ctx, "verify complete", "vm_id", cfg.VMId, "table_exists", result.TableExists, "chain_count", result.ChainCount)
	if err := writeResult(result); err != nil {
		return writeNftError(err)
	}
	return exitSuccess
}

// --- Output helpers ---

// writeParseError writes a JSON parse error response and returns the parse error exit code.
func writeParseError(err error) int {
	slog.Error("parse error", "error", err)
	if encErr := writeResult(&types.NftResult{OK: false, Error: err.Error(), Code: "JSON_PARSE_ERROR"}); encErr != nil {
		slog.Error("failed to write parse error response", "error", encErr)
	}
	return exitParseError
}

// writeValidationError writes a validation error response and returns the parse error exit code.
func writeValidationError(err error) int {
	slog.Error("validation error", "error", err)
	if encErr := writeResult(&types.NftResult{OK: false, Error: err.Error(), Code: "VALIDATION_ERROR"}); encErr != nil {
		slog.Error("failed to write validation error response", "error", encErr)
	}
	return exitParseError
}

// writeNftError classifies an nftables error and writes the appropriate response.
func writeNftError(err error) int {
	if isPermissionError(err) {
		slog.Error("permission denied", "error", err)
		if encErr := writeResult(&types.NftResult{OK: false, Error: err.Error(), Code: "PERMISSION_DENIED"}); encErr != nil {
			slog.Error("failed to write permission error response", "error", encErr)
		}
		return exitPermDenied
	}
	slog.Error("nftables error", "error", err)
	if encErr := writeResult(&types.NftResult{OK: false, Error: err.Error(), Code: "NFTABLES_ERROR"}); encErr != nil {
		slog.Error("failed to write nftables error response", "error", encErr)
	}
	return exitNftError
}

// isPermissionError checks both standard file permission errors and
// netlink EPERM errors (which manifest as syscall.EPERM).
func isPermissionError(err error) bool {
	if os.IsPermission(err) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == syscall.EPERM
}

// --- I/O helpers ---

// readStdin reads all of stdin with a timeout.
func readStdin() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), stdinTimeout)
	defer cancel()

	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)

	go func() {
		data, err := io.ReadAll(os.Stdin)
		ch <- result{data, err}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout reading stdin after %v", stdinTimeout)
	case r := <-ch:
		return r.data, r.err
	}
}

// writeResult encodes a value as JSON to stdout and returns any encoding error.
// Callers should check the returned error and handle appropriately (e.g., return non-zero exit code).
func writeResult(v any) error {
	if r, ok := v.(*types.NftResult); ok && !r.OK {
		slog.Error("operation failed", "error", r.Error, "code", r.Code)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		slog.Error("failed to encode JSON", "error", err)
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}
