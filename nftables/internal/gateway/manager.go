package gateway

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/angelorc/vmsan/nftables/internal/tcpproxy"
)

// VMState holds the runtime state of a single VM's proxy resources.
type VMState struct {
	VMId     string `json:"vmId"`
	Slot     int    `json:"slot"`
	Policy   string `json:"policy"`
	DNSPort  int    `json:"dnsPort"`
	SNIPort  int    `json:"sniPort"`
	HTTPPort int    `json:"httpPort"`
	// Future: dnsproxy PID, tcpproxy listener, config paths
}

// Manager tracks per-VM proxy resources. All methods are safe for
// concurrent use.
type Manager struct {
	mu         sync.RWMutex
	vms        map[string]*VMState
	sniProxies map[string]*tcpproxy.SNIProxy
}

// NewManager returns an empty Manager.
func NewManager() *Manager {
	return &Manager{
		vms:        make(map[string]*VMState),
		sniProxies: make(map[string]*tcpproxy.SNIProxy),
	}
}

// StartVM registers a VM and allocates stub ports. If the VM is already
// registered, the existing state is returned (idempotent).
func (m *Manager) StartVM(vmId string, slot int, policy string, allowedDomains []string) (*VMState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.vms[vmId]; ok {
		slog.Debug("vm already started", "vmId", vmId)
		return existing, nil
	}

	// Port allocation uses the same bases as the TypeScript layer.
	// DNS: 10053+slot, SNI: 10443+slot, HTTP: 10698+slot.
	state := &VMState{
		VMId:     vmId,
		Slot:     slot,
		Policy:   policy,
		DNSPort:  10053 + slot,
		SNIPort:  10443 + slot,
		HTTPPort: 10698 + slot,
	}
	m.vms[vmId] = state

	// Start per-VM SNI proxy.
	sniProxy := tcpproxy.NewSNIProxy(tcpproxy.VmPolicy{
		VMId:           vmId,
		Policy:         policy,
		AllowedDomains: allowedDomains,
	}, fmt.Sprintf("127.0.0.1:%d", state.SNIPort), slog.Default())
	if err := sniProxy.Start(); err != nil {
		slog.Warn("sni proxy start failed", "vmId", vmId, "error", err)
	} else {
		m.sniProxies[vmId] = sniProxy
	}

	slog.Info("vm started", "vmId", vmId, "slot", slot, "policy", policy)
	return state, nil
}

// StopVM removes a VM and releases its resources.
func (m *Manager) StopVM(vmId string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.vms[vmId]; !ok {
		return fmt.Errorf("vm not found: %s", vmId)
	}

	if proxy, ok := m.sniProxies[vmId]; ok {
		if err := proxy.Close(); err != nil {
			slog.Debug("sni proxy close failed", "vmId", vmId, "error", err)
		}
		delete(m.sniProxies, vmId)
	}

	delete(m.vms, vmId)
	slog.Info("vm stopped", "vmId", vmId)
	return nil
}

// UpdatePolicy updates the network policy for a running VM.
func (m *Manager) UpdatePolicy(vmId string, policy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.vms[vmId]
	if !ok {
		return fmt.Errorf("vm not found: %s", vmId)
	}

	state.Policy = policy

	if proxy, ok := m.sniProxies[vmId]; ok {
		if err := proxy.UpdatePolicy(tcpproxy.VmPolicy{
			VMId:   vmId,
			Policy: policy,
		}); err != nil {
			slog.Warn("sni proxy update policy failed", "vmId", vmId, "error", err)
		}
	}

	slog.Info("vm policy updated", "vmId", vmId, "policy", policy)
	return nil
}

// GetVM returns the state for a single VM.
func (m *Manager) GetVM(vmId string) (*VMState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.vms[vmId]
	return state, ok
}

// ListVMs returns a snapshot of all VM states (copies, not internal pointers).
func (m *Manager) ListVMs() []VMState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]VMState, 0, len(m.vms))
	for _, state := range m.vms {
		list = append(list, *state)
	}
	return list
}

// StopAll removes all VMs. Used during graceful shutdown.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for vmId, proxy := range m.sniProxies {
		if err := proxy.Close(); err != nil {
			slog.Debug("sni proxy close failed", "vmId", vmId, "error", err)
		}
	}
	m.sniProxies = make(map[string]*tcpproxy.SNIProxy)

	count := len(m.vms)
	m.vms = make(map[string]*VMState)
	slog.Info("all vms stopped", "count", count)
}
