package firewall

import (
	"fmt"
	"net"
)

// SetupOptions holds all configuration for firewall setup.
// This is the internal programmatic API using functional options.
type SetupOptions struct {
	VMId             string
	Slot             int
	Policy           string
	TapDevice        string
	NetNSName        string
	VmIP             string
	Subnet           string
	GatewayIP        string
	HostIface        string
	HostBridgeIP     string
	HostBridgeSubnet string
	DNS1             string
	DNS2             string
	AllowedPorts     []int
	AllowHostAccess  bool

	// Legacy fields for backward compatibility during migration
	HostIP           string // maps to HostBridgeIP
	GuestIP          string // maps to VmIP
	VethHost         string
	VethGuest        string
	DefaultInterface string // maps to HostIface
	PublishedPorts   []PublishedPort
	AllowedCIDRs     []string
	DeniedCIDRs      []string
	SkipDNAT         bool
	AllowICMP        bool
	DNSResolvers     []string // maps to DNS1, DNS2
}

// SetupOption is a functional option for configuring SetupOptions.
type SetupOption func(*SetupOptions)

// WithSlot sets the VM slot number.
func WithSlot(slot int) SetupOption {
	return func(o *SetupOptions) {
		o.Slot = slot
	}
}

// WithPolicy sets the firewall policy.
func WithPolicy(policy string) SetupOption {
	return func(o *SetupOptions) {
		o.Policy = policy
	}
}

// WithTapDevice sets the tap device name.
func WithTapDevice(device string) SetupOption {
	return func(o *SetupOptions) {
		o.TapDevice = device
	}
}

// WithNetNS sets the network namespace name.
func WithNetNS(name string) SetupOption {
	return func(o *SetupOptions) {
		o.NetNSName = name
	}
}

// WithVmIP sets the VM IP address.
func WithVmIP(ip string) SetupOption {
	return func(o *SetupOptions) {
		o.VmIP = ip
		// Also set legacy field for compatibility
		o.GuestIP = ip
	}
}

// WithSubnet sets the VM subnet.
func WithSubnet(subnet string) SetupOption {
	return func(o *SetupOptions) {
		o.Subnet = subnet
	}
}

// WithGatewayIP sets the gateway IP address.
func WithGatewayIP(ip string) SetupOption {
	return func(o *SetupOptions) {
		o.GatewayIP = ip
	}
}

// WithHostIface sets the host interface.
func WithHostIface(iface string) SetupOption {
	return func(o *SetupOptions) {
		o.HostIface = iface
		// Also set legacy field for compatibility
		o.DefaultInterface = iface
	}
}

// WithHostBridgeIP sets the host bridge IP.
func WithHostBridgeIP(ip string) SetupOption {
	return func(o *SetupOptions) {
		o.HostBridgeIP = ip
		// Also set legacy field for compatibility
		o.HostIP = ip
	}
}

// WithHostBridgeSubnet sets the host bridge subnet.
func WithHostBridgeSubnet(subnet string) SetupOption {
	return func(o *SetupOptions) {
		o.HostBridgeSubnet = subnet
	}
}

// WithDNS sets the DNS resolvers.
func WithDNS(primary, secondary string) SetupOption {
	return func(o *SetupOptions) {
		o.DNS1 = primary
		o.DNS2 = secondary
		// Build legacy DNSResolvers
		o.DNSResolvers = nil
		if primary != "" {
			o.DNSResolvers = append(o.DNSResolvers, primary)
		}
		if secondary != "" {
			o.DNSResolvers = append(o.DNSResolvers, secondary)
		}
	}
}

// WithAllowedPorts sets the allowed ports.
func WithAllowedPorts(ports []int) SetupOption {
	return func(o *SetupOptions) {
		o.AllowedPorts = ports
	}
}

// WithHostAccess sets whether host access is allowed.
func WithHostAccess(allow bool) SetupOption {
	return func(o *SetupOptions) {
		o.AllowHostAccess = allow
	}
}

// WithVethHost sets the veth host interface (legacy compatibility).
func WithVethHost(veth string) SetupOption {
	return func(o *SetupOptions) {
		o.VethHost = veth
	}
}

// WithVethGuest sets the veth guest interface (legacy compatibility).
func WithVethGuest(veth string) SetupOption {
	return func(o *SetupOptions) {
		o.VethGuest = veth
	}
}

// WithPublishedPorts sets the published ports (legacy compatibility).
func WithPublishedPorts(ports []PublishedPort) SetupOption {
	return func(o *SetupOptions) {
		o.PublishedPorts = ports
	}
}

// WithAllowedCIDRs sets the allowed CIDRs (legacy compatibility).
func WithAllowedCIDRs(cidrs []string) SetupOption {
	return func(o *SetupOptions) {
		o.AllowedCIDRs = cidrs
	}
}

// WithDeniedCIDRs sets the denied CIDRs (legacy compatibility).
func WithDeniedCIDRs(cidrs []string) SetupOption {
	return func(o *SetupOptions) {
		o.DeniedCIDRs = cidrs
	}
}

// WithSkipDNAT sets whether to skip DNAT rules.
func WithSkipDNAT(skip bool) SetupOption {
	return func(o *SetupOptions) {
		o.SkipDNAT = skip
	}
}

// NewSetupOptions creates SetupOptions with defaults and applies options.
func NewSetupOptions(vmId string, opts ...SetupOption) *SetupOptions {
	o := &SetupOptions{
		VMId:   vmId,
		Policy: PolicyDenyAll, // default policy
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Validate validates the setup options.
func (o *SetupOptions) Validate() error {
	if o.VMId == "" {
		return ErrMissingVMId
	}
	if o.Policy == "" {
		return ErrMissingPolicy
	}
	switch o.Policy {
	case PolicyAllowAll, PolicyDenyAll, PolicyCustom:
		// valid
	default:
		return &ValidationError{
			Field:   "policy",
			Value:   o.Policy,
			Message: fmt.Sprintf("invalid policy %q: must be %q, %q, or %q", o.Policy, PolicyAllowAll, PolicyDenyAll, PolicyCustom),
		}
	}
	if o.VmIP != "" {
		if ip := net.ParseIP(o.VmIP); ip == nil || ip.To4() == nil {
			return &ValidationError{
				Field:   "vmIp",
				Value:   o.VmIP,
				Message: "must be an IPv4 address",
			}
		}
	}
	if o.HostBridgeIP != "" {
		if ip := net.ParseIP(o.HostBridgeIP); ip == nil || ip.To4() == nil {
			return &ValidationError{
				Field:   "hostBridgeIp",
				Value:   o.HostBridgeIP,
				Message: "must be an IPv4 address",
			}
		}
	}
	if o.GatewayIP != "" {
		if ip := net.ParseIP(o.GatewayIP); ip == nil || ip.To4() == nil {
			return &ValidationError{
				Field:   "gatewayIp",
				Value:   o.GatewayIP,
				Message: "must be an IPv4 address",
			}
		}
	}
	if o.DNS1 != "" {
		if ip := net.ParseIP(o.DNS1); ip == nil || ip.To4() == nil {
			return &ValidationError{
				Field:   "dns1",
				Value:   o.DNS1,
				Message: "must be an IPv4 address",
			}
		}
	}
	if o.DNS2 != "" {
		if ip := net.ParseIP(o.DNS2); ip == nil || ip.To4() == nil {
			return &ValidationError{
				Field:   "dns2",
				Value:   o.DNS2,
				Message: "must be an IPv4 address",
			}
		}
	}

	// Validate legacy fields for compatibility
	for i, pp := range o.PublishedPorts {
		if pp.HostPort == 0 {
			return &ValidationError{
				Field:   fmt.Sprintf("publishedPorts[%d].hostPort", i),
				Message: "is required",
			}
		}
		if pp.GuestPort == 0 {
			return &ValidationError{
				Field:   fmt.Sprintf("publishedPorts[%d].guestPort", i),
				Message: "is required",
			}
		}
		if pp.Protocol != "" && pp.Protocol != "tcp" && pp.Protocol != "udp" {
			return &ValidationError{
				Field:   fmt.Sprintf("publishedPorts[%d].protocol", i),
				Value:   pp.Protocol,
				Message: "must be \"tcp\" or \"udp\"",
			}
		}
		if pp.GuestIP != "" {
			if ip := net.ParseIP(pp.GuestIP); ip == nil || ip.To4() == nil {
				return &ValidationError{
					Field:   fmt.Sprintf("publishedPorts[%d].guestIp", i),
					Value:   pp.GuestIP,
					Message: "must be an IPv4 address",
				}
			}
		}
	}
	for i, cidr := range o.AllowedCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return &ValidationError{
				Field:   fmt.Sprintf("allowedCidrs[%d]", i),
				Value:   cidr,
				Message: fmt.Sprintf("invalid CIDR: %v", err),
			}
		}
	}
	for i, cidr := range o.DeniedCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return &ValidationError{
				Field:   fmt.Sprintf("deniedCidrs[%d]", i),
				Value:   cidr,
				Message: fmt.Sprintf("invalid CIDR: %v", err),
			}
		}
	}
	for i, r := range o.DNSResolvers {
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
