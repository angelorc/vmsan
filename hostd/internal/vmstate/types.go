package vmstate

import (
	vmsanv1 "github.com/angelorc/vmsan/hostd/gen/vmsan/v1"
)

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

// ToProto converts a VmState to its protobuf VMMetadata representation.
func (s *VmState) ToProto() *vmsanv1.VMMetadata {
	m := &vmsanv1.VMMetadata{
		VmId:       s.ID,
		Status:     s.Status,
		Project:    s.Project,
		Runtime:    s.Runtime,
		Vcpus:      int32(s.VcpuCount),
		MemMib:     int32(s.MemSizeMib),
		DiskSizeGb: s.DiskSizeGb,
		CreatedAt:  s.CreatedAt,
		SocketPath: s.APISocket,
		ChrootDir:  s.ChrootDir,
		TapDevice:  s.Network.TapDevice,
		MacAddress: s.Network.MACAddress,
		HostIp:     s.Network.HostIP,
		GuestIp:    s.Network.GuestIP,
		MeshIp:     s.Network.MeshIP,
		SubnetMask: s.Network.SubnetMask,
		NetNsName:  s.Network.NetNSName,
		Service:    s.Network.Service,
		Network:    s.Network.ToNetworkMeta(),
	}

	if s.PID != nil {
		m.Pid = int32(*s.PID)
	}
	if s.TimeoutAt != nil {
		m.TimeoutAt = *s.TimeoutAt
	}
	if s.AgentToken != nil {
		m.AgentToken = *s.AgentToken
	}

	return m
}

// VmStateFromProto creates a VmState from a protobuf VMMetadata message.
func VmStateFromProto(m *vmsanv1.VMMetadata) *VmState {
	s := &VmState{
		ID:         m.VmId,
		Status:     m.Status,
		Project:    m.Project,
		Runtime:    m.Runtime,
		VcpuCount:  int(m.Vcpus),
		MemSizeMib: int(m.MemMib),
		DiskSizeGb: m.DiskSizeGb,
		CreatedAt:  m.CreatedAt,
		APISocket:  m.SocketPath,
		ChrootDir:  m.ChrootDir,
		Network: VmNetwork{
			TapDevice:  m.TapDevice,
			MACAddress: m.MacAddress,
			HostIP:     m.HostIp,
			GuestIP:    m.GuestIp,
			MeshIP:     m.MeshIp,
			SubnetMask: m.SubnetMask,
			NetNSName:  m.NetNsName,
			Service:    m.Service,
		},
	}

	if m.Pid > 0 {
		pid := int(m.Pid)
		s.PID = &pid
	}
	if m.TimeoutAt != "" {
		s.TimeoutAt = &m.TimeoutAt
	}
	if m.AgentToken != "" {
		s.AgentToken = &m.AgentToken
	}

	// Merge network meta if present
	if m.Network != nil {
		s.Network.NetworkPolicy = m.Network.Policy
		s.Network.AllowedDomains = m.Network.Domains
		s.Network.AllowedCidrs = m.Network.AllowedCidrs
		s.Network.DeniedCidrs = m.Network.DeniedCidrs
		s.Network.BandwidthMbit = int(m.Network.BandwidthMbit)
		s.Network.AllowIcmp = m.Network.AllowIcmp
		s.Network.PublishedPorts = fromInt32Slice(m.Network.Ports)
	}

	return s
}

// ToNetworkMeta converts a VmNetwork to its protobuf VMNetworkMeta representation.
func (n *VmNetwork) ToNetworkMeta() *vmsanv1.VMNetworkMeta {
	return &vmsanv1.VMNetworkMeta{
		Policy:        n.NetworkPolicy,
		Domains:       n.AllowedDomains,
		AllowedCidrs:  n.AllowedCidrs,
		DeniedCidrs:   n.DeniedCidrs,
		Ports:         toInt32Slice(n.PublishedPorts),
		BandwidthMbit: int32(n.BandwidthMbit),
		AllowIcmp:     n.AllowIcmp,
	}
}

// VmNetworkFromMeta creates a VmNetwork from a protobuf VMNetworkMeta message.
func VmNetworkFromMeta(m *vmsanv1.VMNetworkMeta) VmNetwork {
	return VmNetwork{
		NetworkPolicy:  m.Policy,
		AllowedDomains: m.Domains,
		AllowedCidrs:   m.AllowedCidrs,
		DeniedCidrs:    m.DeniedCidrs,
		PublishedPorts: fromInt32Slice(m.Ports),
		BandwidthMbit:  int(m.BandwidthMbit),
		AllowIcmp:      m.AllowIcmp,
	}
}

func toInt32Slice(ints []int) []int32 {
	if ints == nil {
		return nil
	}
	out := make([]int32, len(ints))
	for i, v := range ints {
		out[i] = int32(v)
	}
	return out
}

func fromInt32Slice(ints []int32) []int {
	if ints == nil {
		return nil
	}
	out := make([]int, len(ints))
	for i, v := range ints {
		out[i] = int(v)
	}
	return out
}
