package nftables

import (
	"fmt"
	"net"
)

// Policy constants for the firewall setup.
const (
	PolicyAllowAll = "allow-all"
	PolicyDenyAll  = "deny-all"
	PolicyCustom   = "custom"
)

// PublishedPort describes a single port-forwarding rule from host to guest.
type PublishedPort struct {
	HostPort  uint16 `json:"hostPort"`
	GuestIP   string `json:"guestIp"`
	GuestPort uint16 `json:"guestPort"`
	Protocol  string `json:"protocol"` // "tcp" or "udp"
}

// SetupConfig is the JSON input for the "setup" command.
type SetupConfig struct {
	VMId   string `json:"vmId"`
	Slot   int    `json:"slot"`
	Policy string `json:"policy"` // PolicyAllowAll, PolicyDenyAll, PolicyCustom

	// Network topology
	TapDevice        string `json:"tapDevice"`
	HostIP           string `json:"hostIp"`
	GuestIP          string `json:"guestIp"`
	VethHost         string `json:"vethHost"`
	VethGuest        string `json:"vethGuest"`
	NetNSName        string `json:"netnsName"`
	DefaultInterface string `json:"defaultInterface,omitempty"`

	// Firewall rules
	PublishedPorts []PublishedPort `json:"publishedPorts,omitempty"`
	AllowedCIDRs   []string        `json:"allowedCidrs,omitempty"`
	DeniedCIDRs    []string        `json:"deniedCidrs,omitempty"`
	SkipDNAT       bool            `json:"skipDnat,omitempty"`
	DNSResolvers   []string        `json:"dnsResolvers,omitempty"`
}

// Validate checks that required fields are present and values are well-formed.
func (c *SetupConfig) Validate() error {
	if c.VMId == "" {
		return ErrMissingVMId
	}
	if c.Policy == "" {
		return ErrMissingPolicy
	}
	switch c.Policy {
	case PolicyAllowAll, PolicyDenyAll, PolicyCustom:
		// valid
	default:
		return &ValidationError{
			Field:   "policy",
			Value:   c.Policy,
			Message: fmt.Sprintf("invalid policy %q: must be %q, %q, or %q", c.Policy, PolicyAllowAll, PolicyDenyAll, PolicyCustom),
		}
	}
	if c.GuestIP != "" {
		if ip := net.ParseIP(c.GuestIP); ip == nil || ip.To4() == nil {
			return &ValidationError{
				Field:   "guestIp",
				Value:   c.GuestIP,
				Message: "invalid guestIp: must be an IPv4 address",
			}
		}
	}
	if c.HostIP != "" {
		if ip := net.ParseIP(c.HostIP); ip == nil || ip.To4() == nil {
			return &ValidationError{
				Field:   "hostIp",
				Value:   c.HostIP,
				Message: "invalid hostIp: must be an IPv4 address",
			}
		}
	}
	for i, pp := range c.PublishedPorts {
		if pp.HostPort == 0 {
			return &ValidationError{
				Field:   fmt.Sprintf("publishedPorts[%d].hostPort", i),
				Message: "hostPort is required",
			}
		}
		if pp.GuestPort == 0 {
			return &ValidationError{
				Field:   fmt.Sprintf("publishedPorts[%d].guestPort", i),
				Message: "guestPort is required",
			}
		}
		if pp.Protocol != "" && pp.Protocol != "tcp" && pp.Protocol != "udp" {
			return &ValidationError{
				Field:   fmt.Sprintf("publishedPorts[%d].protocol", i),
				Value:   pp.Protocol,
				Message: "protocol must be \"tcp\" or \"udp\"",
			}
		}
		if pp.GuestIP != "" {
			if ip := net.ParseIP(pp.GuestIP); ip == nil || ip.To4() == nil {
				return &ValidationError{
					Field:   fmt.Sprintf("publishedPorts[%d].guestIp", i),
					Value:   pp.GuestIP,
					Message: "invalid guestIp: must be an IPv4 address",
				}
			}
		}
	}
	for i, cidr := range c.AllowedCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return &ValidationError{
				Field:   fmt.Sprintf("allowedCidrs[%d]", i),
				Value:   cidr,
				Message: fmt.Sprintf("invalid CIDR: %v", err),
			}
		}
	}
	for i, cidr := range c.DeniedCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return &ValidationError{
				Field:   fmt.Sprintf("deniedCidrs[%d]", i),
				Value:   cidr,
				Message: fmt.Sprintf("invalid CIDR: %v", err),
			}
		}
	}
	for i, r := range c.DNSResolvers {
		if ip := net.ParseIP(r); ip == nil || ip.To4() == nil {
			return &ValidationError{
				Field:   fmt.Sprintf("dnsResolvers[%d]", i),
				Value:   r,
				Message: "must be an IPv4 address",
			}
		}
	}
	return nil
}

// TeardownConfig is the JSON input for the "teardown" command.
type TeardownConfig struct {
	VMId      string `json:"vmId"`
	NetNSName string `json:"netnsName"`
	// Host-side iptables cleanup fields (0.2.0)
	TapDevice string `json:"tapDevice,omitempty"`
	VethHost  string `json:"vethHost,omitempty"`
	GuestIP   string `json:"guestIp,omitempty"`
	Slot      int    `json:"slot,omitempty"`
}

// Validate checks that required fields are present.
func (c *TeardownConfig) Validate() error {
	if c.VMId == "" {
		return ErrMissingVMId
	}
	return nil
}

// VerifyConfig is the JSON input for the "verify" command.
type VerifyConfig struct {
	VMId      string `json:"vmId"`
	NetNSName string `json:"netnsName"`
}

// Validate checks that required fields are present.
func (c *VerifyConfig) Validate() error {
	if c.VMId == "" {
		return ErrMissingVMId
	}
	return nil
}

// CleanupConfig is the JSON input for the "cleanup-iptables" command.
type CleanupConfig struct {
	VMId      string `json:"vmId"`
	TapDevice string `json:"tapDevice"`
	VethHost  string `json:"vethHost"`
	VethGuest string `json:"vethGuest"`
	NetNSName string `json:"netnsName"`
	HostIP    string `json:"hostIp"`
	GuestIP   string `json:"guestIp"`
}

// Validate checks that required fields are present.
func (c *CleanupConfig) Validate() error {
	if c.VMId == "" {
		return ErrMissingVMId
	}
	return nil
}

// ToOptions converts SetupConfig to SetupOptions for the functional options API.
func (c SetupConfig) ToOptions() *SetupOptions {
	opts := NewSetupOptions(c.VMId)
	opts.Slot = c.Slot
	opts.Policy = c.Policy
	opts.TapDevice = c.TapDevice
	opts.NetNSName = c.NetNSName
	opts.VmIP = c.GuestIP
	opts.HostBridgeIP = c.HostIP
	opts.VethHost = c.VethHost
	opts.VethGuest = c.VethGuest
	opts.HostIface = c.DefaultInterface
	opts.PublishedPorts = c.PublishedPorts
	opts.AllowedCIDRs = c.AllowedCIDRs
	opts.DeniedCIDRs = c.DeniedCIDRs
	opts.SkipDNAT = c.SkipDNAT
	opts.DNSResolvers = c.DNSResolvers
	return opts
}

// ToOptions converts TeardownConfig to TeardownOptions for the functional options API.
func (c TeardownConfig) ToOptions() *TeardownOptions {
	opts := NewTeardownOptions(c.VMId)
	opts.NetNSName = c.NetNSName
	opts.TapDevice = c.TapDevice
	opts.VethHost = c.VethHost
	opts.GuestIP = c.GuestIP
	opts.Slot = c.Slot
	return opts
}

// ToOptions converts VerifyConfig to VerifyOptions for the functional options API.
func (c VerifyConfig) ToOptions() *VerifyOptions {
	opts := NewVerifyOptions(c.VMId)
	opts.NetNSName = c.NetNSName
	return opts
}

// ToOptions converts CleanupConfig to CleanupOptions for the functional options API.
func (c CleanupConfig) ToOptions() *CleanupOptions {
	opts := NewCleanupOptions(c.VMId)
	opts.NetNSName = c.NetNSName
	opts.TapDevice = c.TapDevice
	opts.VethHost = c.VethHost
	opts.VethGuest = c.VethGuest
	opts.HostIP = c.HostIP
	opts.GuestIP = c.GuestIP
	return opts
}

// NftResult is the JSON output for all commands.
type NftResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

// VerifyResult extends NftResult with table/chain inspection data.
type VerifyResult struct {
	NftResult
	TableExists bool `json:"tableExists"`
	ChainCount  int  `json:"chainCount"`
}
