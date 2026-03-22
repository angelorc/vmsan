package netsetup

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	nftypes "github.com/angelorc/vmsan/nftables"
)

// FirewallConfig holds firewall-specific config beyond network setup.
type FirewallConfig struct {
	Policy         string
	AllowedCIDRs   []string
	DeniedCIDRs    []string
	PublishedPorts []nftypes.PublishedPort
	SkipDNAT       bool
	AllowICMP      bool
	DNSResolvers   []string
}

// NftablesBinDir is the directory containing the vmsan-nftables binary.
// If empty, the binary is looked up via $PATH.
var NftablesBinDir string

// SetNftablesBinDir configures the directory containing the vmsan-nftables binary.
func SetNftablesBinDir(dir string) {
	NftablesBinDir = dir
}

// nftablesBinPath returns the resolved path to the vmsan-nftables binary.
func nftablesBinPath() string {
	if NftablesBinDir != "" {
		return filepath.Join(NftablesBinDir, "vmsan-nftables")
	}
	if p, err := exec.LookPath("vmsan-nftables"); err == nil {
		return p
	}
	return "vmsan-nftables"
}

// execNftables executes the vmsan-nftables binary with JSON stdin and returns
// the parsed result.
func execNftables(ctx context.Context, command string, config any) (*nftypes.NftResult, error) {
	binPath := nftablesBinPath()
	input, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal nftables config: %w", err)
	}

	cmd := exec.CommandContext(ctx, binPath, command)
	cmd.Stdin = strings.NewReader(string(input))

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			slog.Debug("vmsan-nftables failed",
				"command", command,
				"exit_code", exitErr.ExitCode(),
				"stderr", string(exitErr.Stderr))
		}

		// Try to parse stdout even on error (binary writes JSON result)
		if len(out) > 0 {
			var result nftypes.NftResult
			if jsonErr := json.Unmarshal(out, &result); jsonErr == nil {
				return &result, fmt.Errorf("vmsan-nftables %s: %s", command, result.Error)
			}
		}
		return nil, fmt.Errorf("vmsan-nftables %s: %w", command, err)
	}

	var result nftypes.NftResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse nftables result: %w", err)
	}

	return &result, nil
}

// SetupFirewall creates per-VM nftables rules by calling vmsan-nftables setup.
func SetupFirewall(ctx context.Context, netCfg SetupConfig, fwCfg FirewallConfig) error {
	cfg := nftypes.SetupConfig{
		VMId:             netCfg.VMId,
		Slot:             netCfg.Slot,
		Policy:           fwCfg.Policy,
		TapDevice:        netCfg.TAPDevice,
		HostIP:           netCfg.HostIP,
		GuestIP:          netCfg.GuestIP,
		VethHost:         netCfg.VethHost,
		VethGuest:        netCfg.VethGuest,
		NetNSName:        netCfg.NetNSName,
		DefaultInterface: netCfg.DefaultIface,
		PublishedPorts:   fwCfg.PublishedPorts,
		AllowedCIDRs:     fwCfg.AllowedCIDRs,
		DeniedCIDRs:      fwCfg.DeniedCIDRs,
		SkipDNAT:         fwCfg.SkipDNAT,
		AllowICMP:        fwCfg.AllowICMP,
		DNSResolvers:     fwCfg.DNSResolvers,
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("firewall setup validation: %w", err)
	}

	result, err := execNftables(ctx, "setup", cfg)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("firewall setup failed: %s", result.Error)
	}
	return nil
}

// TeardownFirewall removes per-VM nftables rules by calling vmsan-nftables teardown.
func TeardownFirewall(ctx context.Context, netCfg SetupConfig, publishedPorts []nftypes.PublishedPort) error {
	cfg := nftypes.TeardownConfig{
		VMId:             netCfg.VMId,
		NetNSName:        netCfg.NetNSName,
		TapDevice:        netCfg.TAPDevice,
		VethHost:         netCfg.VethHost,
		GuestIP:          netCfg.GuestIP,
		Slot:             netCfg.Slot,
		PublishedPorts:   publishedPorts,
		DefaultInterface: netCfg.DefaultIface,
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("firewall teardown validation: %w", err)
	}

	result, err := execNftables(ctx, "teardown", cfg)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("firewall teardown failed: %s", result.Error)
	}
	return nil
}

// VerifyFirewall checks if per-VM nftables table exists by calling vmsan-nftables verify.
func VerifyFirewall(ctx context.Context, vmId, netnsName string) (*nftypes.VerifyResult, error) {
	cfg := nftypes.VerifyConfig{
		VMId:      vmId,
		NetNSName: netnsName,
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("firewall verify validation: %w", err)
	}

	binPath := nftablesBinPath()
	input, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal verify config: %w", err)
	}

	cmd := exec.CommandContext(ctx, binPath, "verify")
	cmd.Stdin = strings.NewReader(string(input))

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("vmsan-nftables verify: %w", err)
	}

	var result nftypes.VerifyResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse verify result: %w", err)
	}

	return &result, nil
}
