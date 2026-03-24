package firewall

// VerifyOptions holds configuration for firewall verification.
type VerifyOptions struct {
	VMId      string
	NetNSName string
}

// VerifyOption is a functional option for configuring VerifyOptions.
type VerifyOption func(*VerifyOptions)

// WithVerifyNetNS sets the network namespace for verification.
func WithVerifyNetNS(name string) VerifyOption {
	return func(o *VerifyOptions) {
		o.NetNSName = name
	}
}

// NewVerifyOptions creates VerifyOptions with defaults and applies options.
func NewVerifyOptions(vmId string, opts ...VerifyOption) *VerifyOptions {
	o := &VerifyOptions{
		VMId: vmId,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Validate checks that required fields are present.
func (o *VerifyOptions) Validate() error {
	if o.VMId == "" {
		return ErrMissingVMId
	}
	return nil
}
