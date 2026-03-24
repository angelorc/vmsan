package vmstate

// VmNetwork holds per-VM network configuration and policy metadata.
type VmNetwork struct {
	TapDevice       string   `json:"tapDevice"`
	HostIP          string   `json:"hostIp"`
	GuestIP         string   `json:"guestIp"`
	SubnetMask      string   `json:"subnetMask"`
	MACAddress      string   `json:"macAddress"`
	NetworkPolicy   string   `json:"networkPolicy"`
	AllowedDomains  []string `json:"allowedDomains"`
	AllowedCidrs    []string `json:"allowedCidrs"`
	DeniedCidrs     []string `json:"deniedCidrs"`
	PublishedPorts  []int    `json:"publishedPorts"`
	TunnelHostname  *string  `json:"tunnelHostname"`
	TunnelHostnames []string `json:"tunnelHostnames,omitempty"`
	BandwidthMbit   int      `json:"bandwidthMbit,omitempty"`
	NetNSName       string   `json:"netnsName,omitempty"`
	SkipDnat        bool     `json:"skipDnat,omitempty"`
	AllowIcmp       bool     `json:"allowIcmp,omitempty"`
	FirewallBackend string   `json:"firewallBackend,omitempty"`
	ConnectTo       []string `json:"connectTo,omitempty"`
	MeshIP          string   `json:"meshIp,omitempty"`
	Service         string   `json:"service,omitempty"`
}

// VmState is the local JSON state for a single VM, matching the TypeScript VmState type.
type VmState struct {
	ID             string    `json:"id"`
	Project        string    `json:"project"`
	Runtime        string    `json:"runtime"`
	DiskSizeGb     float64   `json:"diskSizeGb,omitempty"`
	Status         string    `json:"status"` // "creating", "running", "stopped", "error"
	PID            *int      `json:"pid"`
	APISocket      string    `json:"apiSocket"`
	ChrootDir      string    `json:"chrootDir"`
	Kernel         string    `json:"kernel"`
	Rootfs         string    `json:"rootfs"`
	VcpuCount      int       `json:"vcpuCount"`
	MemSizeMib     int       `json:"memSizeMib"`
	Network        VmNetwork `json:"network"`
	Snapshot       *string   `json:"snapshot"`
	TimeoutMs      *int64    `json:"timeoutMs"`
	TimeoutAt      *string   `json:"timeoutAt"`
	CreatedAt      string    `json:"createdAt"`
	Error          *string   `json:"error"`
	AgentToken     *string   `json:"agentToken"`
	AgentPort      int       `json:"agentPort"`
	StateVersion   int       `json:"stateVersion"`
	DisableSeccomp bool      `json:"disableSeccomp,omitempty"`
	DisablePidNs   bool      `json:"disablePidNs,omitempty"`
	DisableCgroup  bool      `json:"disableCgroup,omitempty"`
}
