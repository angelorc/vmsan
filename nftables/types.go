package nftables

import (
	"errors"
	"fmt"
	"net"
)

// Policy constants for the firewall setup.
const (
	PolicyAllowAll = "allow-all"
	PolicyDenyAll  = "deny-all"
	PolicyCustom   = "custom"
)

// Sentinel errors for input validation.
var (
	ErrMissingVMId = errors.New("vmId is required")
	ErrMissingPolicy = errors.New("policy is required")
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
	AllowedCIDRs   []string       `json:"allowedCidrs,omitempty"`
	DeniedCIDRs    []string       `json:"deniedCidrs,omitempty"`
	SkipDNAT       bool           `json:"skipDnat,omitempty"`
	DNSResolvers   []string       `json:"dnsResolvers,omitempty"`
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
		return fmt.Errorf("invalid policy %q: must be %q, %q, or %q", c.Policy, PolicyAllowAll, PolicyDenyAll, PolicyCustom)
	}
	if c.GuestIP != "" {
		if ip := net.ParseIP(c.GuestIP); ip == nil || ip.To4() == nil {
			return fmt.Errorf("invalid guestIp %q: must be an IPv4 address", c.GuestIP)
		}
	}
	if c.HostIP != "" {
		if ip := net.ParseIP(c.HostIP); ip == nil || ip.To4() == nil {
			return fmt.Errorf("invalid hostIp %q: must be an IPv4 address", c.HostIP)
		}
	}
	for i, pp := range c.PublishedPorts {
		if pp.HostPort == 0 {
			return fmt.Errorf("publishedPorts[%d]: hostPort is required", i)
		}
		if pp.GuestPort == 0 {
			return fmt.Errorf("publishedPorts[%d]: guestPort is required", i)
		}
		if pp.Protocol != "" && pp.Protocol != "tcp" && pp.Protocol != "udp" {
			return fmt.Errorf("publishedPorts[%d]: protocol must be \"tcp\" or \"udp\", got %q", i, pp.Protocol)
		}
		if pp.GuestIP != "" {
			if ip := net.ParseIP(pp.GuestIP); ip == nil || ip.To4() == nil {
				return fmt.Errorf("publishedPorts[%d]: invalid guestIp %q: must be an IPv4 address", i, pp.GuestIP)
			}
		}
	}
	for i, cidr := range c.AllowedCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("allowedCidrs[%d] %q: %w", i, cidr, err)
		}
	}
	for i, cidr := range c.DeniedCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("deniedCidrs[%d] %q: %w", i, cidr, err)
		}
	}
	for i, r := range c.DNSResolvers {
		if ip := net.ParseIP(r); ip == nil || ip.To4() == nil {
			return fmt.Errorf("dnsResolvers[%d] %q: must be an IPv4 address", i, r)
		}
	}
	return nil
}

// TeardownConfig is the JSON input for the "teardown" command.
type TeardownConfig struct {
	VMId      string `json:"vmId"`
	NetNSName string `json:"netnsName"`
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
