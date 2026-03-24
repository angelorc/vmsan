package netsetup

import (
	"context"
	"fmt"

	"github.com/angelorc/vmsan/hostd/internal/firewall"
)

// FirewallConfig holds firewall-specific config beyond network setup.
type FirewallConfig struct {
	Policy         string
	AllowedCIDRs   []string
	DeniedCIDRs    []string
	PublishedPorts []firewall.PublishedPort
	SkipDNAT       bool
	AllowICMP      bool
	DNSResolvers   []string
}

// SetupFirewall creates per-VM nftables rules by calling the firewall package directly.
func SetupFirewall(ctx context.Context, netCfg SetupConfig, fwCfg FirewallConfig) error {
	cfg := firewall.SetupConfig{
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

	opts := cfg.ToOptions()
	return firewall.Setup(ctx, opts)
}

// TeardownFirewall removes per-VM nftables rules by calling the firewall package directly.
func TeardownFirewall(ctx context.Context, netCfg SetupConfig, publishedPorts []firewall.PublishedPort) error {
	cfg := firewall.TeardownConfig{
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

	opts := cfg.ToOptions()
	return firewall.Teardown(ctx, opts)
}

// VerifyFirewall checks if per-VM nftables table exists by calling the firewall package directly.
func VerifyFirewall(ctx context.Context, vmId, netnsName string) (*firewall.VerifyResult, error) {
	cfg := firewall.VerifyConfig{
		VMId:      vmId,
		NetNSName: netnsName,
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("firewall verify validation: %w", err)
	}

	opts := cfg.ToOptions()
	return firewall.Verify(ctx, opts)
}
