package compat

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// IptablesExecutor defines the interface for executing iptables commands.
// This abstraction allows for dependency injection and easier testing.
type IptablesExecutor interface {
	Execute(ctx context.Context, args ...string) (string, error)
	Save(ctx context.Context, netns string) (string, error)
	Restore(ctx context.Context, netns, rules string) error
}

// RealIptablesExecutor is the production implementation of IptablesExecutor.
type RealIptablesExecutor struct{}

// Compile-time check: ensure RealIptablesExecutor implements IptablesExecutor.
var _ IptablesExecutor = (*RealIptablesExecutor)(nil)

// NewRealIptablesExecutor creates a new real iptables executor.
func NewRealIptablesExecutor() *RealIptablesExecutor {
	return &RealIptablesExecutor{}
}

// Execute runs an iptables command with the given arguments.
// Returns the combined output (stdout + stderr) and any error.
func (e *RealIptablesExecutor) Execute(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "iptables", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("iptables %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Save runs iptables-save and returns the output.
// If netns is non-empty, runs inside the network namespace.
func (e *RealIptablesExecutor) Save(ctx context.Context, netns string) (string, error) {
	var cmd *exec.Cmd
	if netns != "" {
		cmd = exec.CommandContext(ctx, "ip", "netns", "exec", netns, "iptables-save")
	} else {
		cmd = exec.CommandContext(ctx, "iptables-save")
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("iptables-save: %w", err)
	}
	return string(out), nil
}

// Restore runs iptables-restore with the given rules.
// If netns is non-empty, runs inside the network namespace.
func (e *RealIptablesExecutor) Restore(ctx context.Context, netns, rules string) error {
	var cmd *exec.Cmd
	if netns != "" {
		cmd = exec.CommandContext(ctx, "ip", "netns", "exec", netns, "iptables-restore")
	} else {
		cmd = exec.CommandContext(ctx, "iptables-restore")
	}
	cmd.Stdin = strings.NewReader(rules)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("iptables-restore: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
