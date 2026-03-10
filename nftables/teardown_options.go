package nftables

// TeardownOptions holds configuration for firewall teardown.
type TeardownOptions struct {
	VMId      string
	NetNSName string

	// Legacy fields for backward compatibility
	TapDevice string
	VethHost  string
	GuestIP   string
	Slot      int

	// Published ports for DNAT rule cleanup
	PublishedPorts []PublishedPort
	HostIface      string
}

// TeardownOption is a functional option for configuring TeardownOptions.
type TeardownOption func(*TeardownOptions)

// WithTeardownNetNS sets the network namespace for teardown.
func WithTeardownNetNS(name string) TeardownOption {
	return func(o *TeardownOptions) {
		o.NetNSName = name
	}
}

// WithTeardownTapDevice sets the tap device for teardown (legacy compatibility).
func WithTeardownTapDevice(device string) TeardownOption {
	return func(o *TeardownOptions) {
		o.TapDevice = device
	}
}

// WithTeardownVethHost sets the veth host interface for teardown (legacy compatibility).
func WithTeardownVethHost(veth string) TeardownOption {
	return func(o *TeardownOptions) {
		o.VethHost = veth
	}
}

// WithTeardownGuestIP sets the guest IP for teardown (legacy compatibility).
func WithTeardownGuestIP(ip string) TeardownOption {
	return func(o *TeardownOptions) {
		o.GuestIP = ip
	}
}

// WithTeardownSlot sets the slot for teardown (legacy compatibility).
func WithTeardownSlot(slot int) TeardownOption {
	return func(o *TeardownOptions) {
		o.Slot = slot
	}
}

// WithTeardownPublishedPorts sets the published ports for DNAT rule cleanup.
func WithTeardownPublishedPorts(ports []PublishedPort) TeardownOption {
	return func(o *TeardownOptions) {
		o.PublishedPorts = ports
	}
}

// WithTeardownHostIface sets the host interface for teardown.
func WithTeardownHostIface(iface string) TeardownOption {
	return func(o *TeardownOptions) {
		o.HostIface = iface
	}
}

// NewTeardownOptions creates TeardownOptions with defaults and applies options.
func NewTeardownOptions(vmId string, opts ...TeardownOption) *TeardownOptions {
	o := &TeardownOptions{
		VMId: vmId,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Validate checks that required fields are present.
func (o *TeardownOptions) Validate() error {
	if o.VMId == "" {
		return ErrMissingVMId
	}
	return nil
}
