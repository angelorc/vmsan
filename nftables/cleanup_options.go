package nftables

// CleanupOptions holds configuration for legacy iptables cleanup.
type CleanupOptions struct {
	VMId   string
	DryRun bool

	// Legacy fields for backward compatibility
	NetNSName string
	TapDevice string
	VethHost  string
	VethGuest string
	HostIP    string
	GuestIP   string
}

// CleanupOption is a functional option for configuring CleanupOptions.
type CleanupOption func(*CleanupOptions)

// WithDryRun sets dry-run mode.
func WithDryRun(dry bool) CleanupOption {
	return func(o *CleanupOptions) {
		o.DryRun = dry
	}
}

// WithCleanupNetNS sets the network namespace for cleanup.
func WithCleanupNetNS(name string) CleanupOption {
	return func(o *CleanupOptions) {
		o.NetNSName = name
	}
}

// WithCleanupTapDevice sets the tap device for cleanup (legacy compatibility).
func WithCleanupTapDevice(device string) CleanupOption {
	return func(o *CleanupOptions) {
		o.TapDevice = device
	}
}

// WithCleanupVethHost sets the veth host interface for cleanup (legacy compatibility).
func WithCleanupVethHost(veth string) CleanupOption {
	return func(o *CleanupOptions) {
		o.VethHost = veth
	}
}

// WithCleanupVethGuest sets the veth guest interface for cleanup (legacy compatibility).
func WithCleanupVethGuest(veth string) CleanupOption {
	return func(o *CleanupOptions) {
		o.VethGuest = veth
	}
}

// WithCleanupHostIP sets the host IP for cleanup (legacy compatibility).
func WithCleanupHostIP(ip string) CleanupOption {
	return func(o *CleanupOptions) {
		o.HostIP = ip
	}
}

// WithCleanupGuestIP sets the guest IP for cleanup (legacy compatibility).
func WithCleanupGuestIP(ip string) CleanupOption {
	return func(o *CleanupOptions) {
		o.GuestIP = ip
	}
}

// NewCleanupOptions creates CleanupOptions with defaults and applies options.
func NewCleanupOptions(vmId string, opts ...CleanupOption) *CleanupOptions {
	o := &CleanupOptions{
		VMId: vmId,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Validate checks that required fields are present.
func (o *CleanupOptions) Validate() error {
	if o.VMId == "" {
		return ErrMissingVMId
	}
	return nil
}
