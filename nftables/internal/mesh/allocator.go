// Package mesh manages inter-VM mesh networking: IP allocation, DNS resolution,
// L3 routing, and nftables ACLs for project-scoped VM communication.
package mesh

import (
	"fmt"
	"sync"
)

// MeshCIDR is the default mesh IP range.
const MeshCIDR = "10.90.0.0/16"

// maxProjectIndex is the maximum project index (10.90.255.x).
const maxProjectIndex = 255

// maxVMIndex is the maximum VM index within a project (10.90.x.254).
// .0 is the network address, .255 is broadcast, so valid range is .1-.254.
const maxVMIndex = 253

// Allocator manages mesh IP assignments per project.
type Allocator struct {
	mu          sync.RWMutex
	assignments map[string]*ProjectAllocation // projectName -> allocation
	vmIndex     map[string]string             // vmId -> projectName (reverse lookup)
	cidr        string
	nextProject int // next project index to assign
}

// ProjectAllocation tracks mesh IP assignments for a single project.
type ProjectAllocation struct {
	ProjectIndex int
	VMs          map[string]MeshIPAssignment // vmId -> assignment
	nextVM       int                         // next VM index to assign
	freeVMs      []int                       // recycled VM indices
}

// MeshIPAssignment represents a single VM's mesh IP assignment.
type MeshIPAssignment struct {
	VMId    string `json:"vmId"`
	VMIndex int    `json:"vmIndex"`
	MeshIP  string `json:"meshIp"`
	Service string `json:"service,omitempty"`
	Project string `json:"project"`
}

// NewAllocator creates a new mesh IP allocator. If cidr is empty, MeshCIDR is used.
func NewAllocator(cidr string) *Allocator {
	if cidr == "" {
		cidr = MeshCIDR
	}
	return &Allocator{
		assignments: make(map[string]*ProjectAllocation),
		vmIndex:     make(map[string]string),
		cidr:        cidr,
	}
}

// Allocate assigns a mesh IP to a VM within a project.
// If the VM already has an allocation, it returns the existing one.
func (a *Allocator) Allocate(project string, vmId string, service string) (MeshIPAssignment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if VM already has an allocation
	if existingProject, ok := a.vmIndex[vmId]; ok {
		pa := a.assignments[existingProject]
		if assignment, found := pa.VMs[vmId]; found {
			return assignment, nil
		}
	}

	// Get or create project allocation
	pa, err := a.getOrCreateProject(project)
	if err != nil {
		return MeshIPAssignment{}, err
	}

	// Assign VM index — reuse freed indices first, then bump counter.
	var vmIdx int
	if len(pa.freeVMs) > 0 {
		vmIdx = pa.freeVMs[len(pa.freeVMs)-1]
		pa.freeVMs = pa.freeVMs[:len(pa.freeVMs)-1]
	} else {
		vmIdx = pa.nextVM
		if vmIdx > maxVMIndex {
			return MeshIPAssignment{}, fmt.Errorf("project %q has reached maximum VM count (%d)", project, maxVMIndex+1)
		}
		pa.nextVM++
	}

	// Build mesh IP: 10.90.{projectIndex}.{vmIndex+1}
	meshIP := fmt.Sprintf("10.90.%d.%d", pa.ProjectIndex, vmIdx+1)

	assignment := MeshIPAssignment{
		VMId:    vmId,
		VMIndex: vmIdx,
		MeshIP:  meshIP,
		Service: service,
		Project: project,
	}

	pa.VMs[vmId] = assignment
	a.vmIndex[vmId] = project

	return assignment, nil
}

// Release removes a VM's mesh IP assignment.
func (a *Allocator) Release(vmId string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	project, ok := a.vmIndex[vmId]
	if !ok {
		return fmt.Errorf("VM %q has no mesh IP assignment", vmId)
	}

	pa := a.assignments[project]
	if assignment, ok := pa.VMs[vmId]; ok {
		pa.freeVMs = append(pa.freeVMs, assignment.VMIndex)
	}
	delete(pa.VMs, vmId)
	delete(a.vmIndex, vmId)

	return nil
}

// GetByVMId returns the mesh IP assignment for a VM.
func (a *Allocator) GetByVMId(vmId string) (MeshIPAssignment, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	project, ok := a.vmIndex[vmId]
	if !ok {
		return MeshIPAssignment{}, false
	}

	pa := a.assignments[project]
	assignment, found := pa.VMs[vmId]
	return assignment, found
}

// GetByService returns the mesh IP assignment for a service within a project.
func (a *Allocator) GetByService(project string, service string) (MeshIPAssignment, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	pa, ok := a.assignments[project]
	if !ok {
		return MeshIPAssignment{}, false
	}

	for _, assignment := range pa.VMs {
		if assignment.Service == service {
			return assignment, true
		}
	}

	return MeshIPAssignment{}, false
}

// ListByProject returns all mesh IP assignments for a project.
func (a *Allocator) ListByProject(project string) []MeshIPAssignment {
	a.mu.RLock()
	defer a.mu.RUnlock()

	pa, ok := a.assignments[project]
	if !ok {
		return nil
	}

	result := make([]MeshIPAssignment, 0, len(pa.VMs))
	for _, assignment := range pa.VMs {
		result = append(result, assignment)
	}

	return result
}

// ProjectIndex returns the index assigned to a project, or -1 if the project
// has no allocations.
func (a *Allocator) ProjectIndex(project string) int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	pa, ok := a.assignments[project]
	if !ok {
		return -1
	}
	return pa.ProjectIndex
}

// getOrCreateProject returns the ProjectAllocation for a project, creating one
// if it doesn't exist. Must be called with a.mu held.
func (a *Allocator) getOrCreateProject(project string) (*ProjectAllocation, error) {
	if pa, ok := a.assignments[project]; ok {
		return pa, nil
	}

	if a.nextProject > maxProjectIndex {
		return nil, fmt.Errorf("maximum number of projects (%d) reached", maxProjectIndex+1)
	}

	pa := &ProjectAllocation{
		ProjectIndex: a.nextProject,
		VMs:          make(map[string]MeshIPAssignment),
	}
	a.nextProject++
	a.assignments[project] = pa

	return pa, nil
}
