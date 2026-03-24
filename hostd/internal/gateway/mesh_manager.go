// Package gateway provides the mesh networking manager that coordinates
// IP allocation, DNS registration, routing, and firewall ACLs for inter-VM
// mesh communication.
package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/angelorc/vmsan/hostd/internal/mesh"
)

// Manager coordinates mesh networking lifecycle for VMs.
// It wires together the allocator, DNS handler, routing, and firewall ACLs.
type MeshManager struct {
	allocator *mesh.Allocator
	dns       *mesh.DNSHandler
	firewall  *mesh.MeshFirewall
	logger    *slog.Logger
	slots     *SlotAllocator

	mu         sync.Mutex
	started    bool
	cancel     context.CancelFunc
	connectMap map[string][]string // vmId → connectTo list for retroactive ACL setup
}

// NewManager creates a new mesh gateway manager.
func NewMeshManager(logger *slog.Logger, slots *SlotAllocator) *MeshManager {
	if logger == nil {
		logger = slog.Default()
	}
	allocator := mesh.NewAllocator("")
	return &MeshManager{
		allocator: allocator,
		dns:       mesh.NewDNSHandler(allocator, 0, logger),
		firewall:  mesh.NewMeshFirewall(logger),
		logger:    logger,
		slots:     slots,
	}
}

// Start initializes the mesh firewall and starts the DNS server.
func (m *MeshManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}

	dnsCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	if err := m.firewall.Init(); err != nil {
		cancel()
		return fmt.Errorf("init mesh firewall: %w", err)
	}

	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	// Start DNS server in background.
	go func() {
		if err := m.dns.Start(dnsCtx); err != nil {
			m.logger.Error("mesh DNS server error", "error", err.Error())
		}
	}()

	m.logger.Info("mesh gateway manager started")
	return nil
}

// Stop shuts down the DNS server and cleans up the mesh firewall.
func (m *MeshManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}
	m.started = false

	if m.cancel != nil {
		m.cancel()
	}

	if err := m.dns.Stop(); err != nil {
		m.logger.Debug("mesh DNS stop error", "error", err.Error())
	}

	if err := m.firewall.Cleanup(); err != nil {
		m.logger.Debug("mesh firewall cleanup error", "error", err.Error())
	}

	m.logger.Info("mesh gateway manager stopped")
	return nil
}

// VMStartParams holds the parameters for registering a VM with the mesh.
type VMStartParams struct {
	VMId      string   `json:"vmId"`
	Slot      int      `json:"slot"`
	Policy    string   `json:"policy"`
	Project   string   `json:"project"`
	Service   string   `json:"service"`
	ConnectTo []string `json:"connectTo"` // e.g., ["postgres:5432", "redis:6379"]
	VethHost  string   `json:"vethHost"`
	NetNS     string   `json:"netns"`
	GuestDev  string   `json:"guestDev"`
	TAPDevice string   `json:"tapDevice"`
}

// VMStartResult holds the result of registering a VM with the mesh.
type VMStartResult struct {
	MeshIP  string                 `json:"meshIp"`
	Service string                 `json:"service,omitempty"`
	Peers   []mesh.MeshIPAssignment `json:"peers,omitempty"`
}

// slotByVMId returns the slot for a VM, or -1 if not found.
func (m *MeshManager) slotByVMId(vmId string) int {
	if m.slots == nil {
		return -1
	}
	return m.slots.GetSlot(vmId)
}

// RegisterDNSVM registers a VM's guest IP for DNS query logging and policy filtering.
func (m *MeshManager) RegisterDNSVM(vmId, guestIP, policy string) {
	m.dns.RegisterVM(vmId, guestIP, policy)
}

// UnregisterDNSVM removes a VM's transit IP from DNS logging.
func (m *MeshManager) UnregisterDNSVM(transitGuestIP string) {
	m.dns.UnregisterVM(transitGuestIP)
}

// OnVMStart allocates a mesh IP, sets up routes, registers in DNS, and
// configures ACLs for the VM's connectTo peers.
func (m *MeshManager) OnVMStart(params VMStartParams) (*VMStartResult, error) {
	// Allocate mesh IP.
	assignment, err := m.allocator.Allocate(params.Project, params.VMId, params.Service)
	if err != nil {
		return nil, fmt.Errorf("allocate mesh IP: %w", err)
	}

	m.logger.Info("mesh IP allocated",
		"vmId", params.VMId,
		"meshIp", assignment.MeshIP,
		"project", params.Project,
		"service", params.Service,
	)

	// Add host-side route via transit guest IP.
	if params.VethHost != "" && params.Slot >= 0 {
		transitGuestIP := fmt.Sprintf("10.200.%d.2", params.Slot)
		if err := mesh.AddRoute(assignment.MeshIP, params.VethHost, transitGuestIP); err != nil {
			m.logger.Debug("failed to add mesh route (may already exist)", "error", err.Error())
		}
	}

	// Set up ARP + DNAT so mesh traffic reaching this VM is forwarded to the guest.
	if params.NetNS != "" && params.Slot >= 0 && params.GuestDev != "" {
		guestIP := fmt.Sprintf("198.19.%d.2", params.Slot)
		m.logger.Info("setting up mesh routing",
			"vmId", params.VMId, "meshIP", assignment.MeshIP,
			"guestIP", guestIP, "vethGuest", params.GuestDev,
			"netns", params.NetNS, "slot", params.Slot)
		if err := mesh.AddGuestRoute(params.NetNS, assignment.MeshIP, guestIP, params.GuestDev); err != nil {
			m.logger.Error("failed to add mesh routing", "vmId", params.VMId, "error", err.Error())
		}
	}

	// Store connectTo for retroactive ACL setup when peers start later.
	if len(params.ConnectTo) > 0 {
		m.mu.Lock()
		if m.connectMap == nil {
			m.connectMap = make(map[string][]string)
		}
		m.connectMap[params.VMId] = params.ConnectTo
		m.mu.Unlock()

		if err := m.setupConnections(params.Project, assignment.MeshIP, params.ConnectTo); err != nil {
			m.logger.Debug("failed to setup mesh connections", "error", err.Error())
		}
	}

	// Retroactively add ACLs for peers that started before us and had
	// connectTo entries targeting our service (which failed because we
	// didn't exist yet).
	if params.Service != "" {
		m.setupPendingConnections(params.Project, params.Service, assignment.MeshIP)
	}

	// List current project peers.
	peers := m.allocator.ListByProject(params.Project)

	return &VMStartResult{
		MeshIP:  assignment.MeshIP,
		Service: assignment.Service,
		Peers:   peers,
	}, nil
}

// OnVMStop removes routes, DNS registration, and ACLs for the VM.
func (m *MeshManager) OnVMStop(vmId string, vethHost string, netNS string, guestDev string) error {
	assignment, ok := m.allocator.GetByVMId(vmId)
	if !ok {
		return nil // VM has no mesh allocation, nothing to do.
	}

	m.logger.Info("releasing mesh IP",
		"vmId", vmId,
		"meshIp", assignment.MeshIP,
		"project", assignment.Project,
	)

	// Remove connectTo tracking.
	m.mu.Lock()
	delete(m.connectMap, vmId)
	m.mu.Unlock()

	// Remove mesh ACL entries for this VM.
	if err := m.firewall.RemoveVM(assignment.MeshIP); err != nil {
		m.logger.Debug("failed to remove mesh ACL entries", "error", err.Error())
	}

	// Remove guest mesh address.
	if netNS != "" && guestDev != "" {
		if err := mesh.RemoveGuestRoute(netNS, assignment.MeshIP, guestDev); err != nil {
			m.logger.Debug("failed to remove guest mesh addr", "error", err.Error())
		}
	}

	// Remove host-side route.
	if err := mesh.RemoveRoute(assignment.MeshIP); err != nil {
		m.logger.Debug("failed to remove mesh route", "error", err.Error())
	}

	// Release allocation (also removes from DNS lookups).
	if err := m.allocator.Release(vmId); err != nil {
		return fmt.Errorf("release mesh IP: %w", err)
	}

	return nil
}

// Allocator returns the underlying mesh IP allocator.
func (m *MeshManager) Allocator() *mesh.Allocator {
	return m.allocator
}

// setupPendingConnections checks existing peers' connectTo lists for entries
// targeting the newly started service. If peer A started with --connect-to
// "db:5432" but the db VM didn't exist yet, the ACL was skipped. When db
// starts, this method retroactively creates the ACL.
func (m *MeshManager) setupPendingConnections(project, newService, newMeshIP string) {
	m.mu.Lock()
	snapshot := make(map[string][]string, len(m.connectMap))
	for k, v := range m.connectMap {
		snapshot[k] = v
	}
	m.mu.Unlock()

	for peerVMId, peerConnectTo := range snapshot {
		peerAssignment, ok := m.allocator.GetByVMId(peerVMId)
		if !ok || peerAssignment.Project != project {
			continue
		}
		for _, conn := range peerConnectTo {
			service, port, err := parseConnectTo(conn)
			if err != nil || service != newService {
				continue
			}
			entries := []mesh.ACLEntry{
				{SrcIP: peerAssignment.MeshIP, DstIP: newMeshIP, DstPort: port, Proto: "tcp"},
				{SrcIP: newMeshIP, DstIP: peerAssignment.MeshIP, DstPort: port, Proto: "tcp"},
			}
			if err := m.firewall.AllowMesh(entries); err != nil {
				m.logger.Debug("failed to add pending mesh ACL", "peer", peerVMId, "error", err.Error())
			} else {
				m.logger.Info("retroactive mesh ACL added",
					"peer", peerVMId, "service", newService, "port", port)
			}
		}
	}
}

// setupConnections creates bidirectional mesh ACL entries for the given
// connectTo services. Each entry in connectTo is "service:port".
func (m *MeshManager) setupConnections(project string, srcIP string, connectTo []string) error {
	var entries []mesh.ACLEntry

	for _, conn := range connectTo {
		service, port, err := parseConnectTo(conn)
		if err != nil {
			return fmt.Errorf("parse connectTo %q: %w", conn, err)
		}

		peer, found := m.allocator.GetByService(project, service)
		if !found {
			m.logger.Debug("connectTo peer not found (may start later)", "service", service, "project", project)
			continue
		}

		// Allow src -> dst on the specified port (TCP by default).
		entries = append(entries, mesh.ACLEntry{
			SrcIP:   srcIP,
			DstIP:   peer.MeshIP,
			DstPort: port,
			Proto:   "tcp",
		})

		// Allow return traffic: dst -> src (established connections handled by conntrack,
		// but we also allow the peer to initiate connections back).
		entries = append(entries, mesh.ACLEntry{
			SrcIP:   peer.MeshIP,
			DstIP:   srcIP,
			DstPort: port,
			Proto:   "tcp",
		})

	}

	if len(entries) == 0 {
		return nil
	}

	return m.firewall.AllowMesh(entries)
}

// parseConnectTo parses a "service:port" string.
func parseConnectTo(s string) (string, uint16, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("expected \"service:port\" format")
	}

	service := parts[0]
	if service == "" {
		return "", 0, fmt.Errorf("service name cannot be empty")
	}

	port, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q: %w", parts[1], err)
	}
	if port == 0 {
		return "", 0, fmt.Errorf("port must be non-zero")
	}

	return service, uint16(port), nil
}
