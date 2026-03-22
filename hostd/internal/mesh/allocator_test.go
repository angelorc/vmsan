package mesh

import (
	"sync"
	"testing"
)

func TestAllocatorBasic(t *testing.T) {
	a := NewAllocator("")

	// First allocation in a project should get index 0, VM index 0 -> IP 10.90.0.1
	assignment, err := a.Allocate("myproject", "vm-1", "web")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if assignment.MeshIP != "10.90.0.1" {
		t.Errorf("MeshIP = %q, want %q", assignment.MeshIP, "10.90.0.1")
	}
	if assignment.VMId != "vm-1" {
		t.Errorf("VMId = %q, want %q", assignment.VMId, "vm-1")
	}
	if assignment.Service != "web" {
		t.Errorf("Service = %q, want %q", assignment.Service, "web")
	}
	if assignment.Project != "myproject" {
		t.Errorf("Project = %q, want %q", assignment.Project, "myproject")
	}
	if assignment.VMIndex != 0 {
		t.Errorf("VMIndex = %d, want %d", assignment.VMIndex, 0)
	}
}

func TestAllocatorMultipleVMs(t *testing.T) {
	a := NewAllocator("")

	a1, err := a.Allocate("proj", "vm-1", "web")
	if err != nil {
		t.Fatalf("Allocate vm-1: %v", err)
	}
	a2, err := a.Allocate("proj", "vm-2", "db")
	if err != nil {
		t.Fatalf("Allocate vm-2: %v", err)
	}
	a3, err := a.Allocate("proj", "vm-3", "cache")
	if err != nil {
		t.Fatalf("Allocate vm-3: %v", err)
	}

	if a1.MeshIP != "10.90.0.1" {
		t.Errorf("vm-1 MeshIP = %q, want %q", a1.MeshIP, "10.90.0.1")
	}
	if a2.MeshIP != "10.90.0.2" {
		t.Errorf("vm-2 MeshIP = %q, want %q", a2.MeshIP, "10.90.0.2")
	}
	if a3.MeshIP != "10.90.0.3" {
		t.Errorf("vm-3 MeshIP = %q, want %q", a3.MeshIP, "10.90.0.3")
	}
}

func TestAllocatorMultipleProjects(t *testing.T) {
	a := NewAllocator("")

	a1, err := a.Allocate("alpha", "vm-a1", "web")
	if err != nil {
		t.Fatalf("Allocate alpha/vm-a1: %v", err)
	}
	a2, err := a.Allocate("beta", "vm-b1", "api")
	if err != nil {
		t.Fatalf("Allocate beta/vm-b1: %v", err)
	}

	if a1.MeshIP != "10.90.0.1" {
		t.Errorf("alpha/vm-a1 MeshIP = %q, want %q", a1.MeshIP, "10.90.0.1")
	}
	if a2.MeshIP != "10.90.1.1" {
		t.Errorf("beta/vm-b1 MeshIP = %q, want %q", a2.MeshIP, "10.90.1.1")
	}
}

func TestAllocatorProjectIndex(t *testing.T) {
	a := NewAllocator("")

	// No allocations yet.
	if idx := a.ProjectIndex("unknown"); idx != -1 {
		t.Errorf("ProjectIndex for unknown = %d, want -1", idx)
	}

	a.Allocate("first", "vm-1", "")
	a.Allocate("second", "vm-2", "")
	a.Allocate("third", "vm-3", "")

	if idx := a.ProjectIndex("first"); idx != 0 {
		t.Errorf("ProjectIndex for first = %d, want 0", idx)
	}
	if idx := a.ProjectIndex("second"); idx != 1 {
		t.Errorf("ProjectIndex for second = %d, want 1", idx)
	}
	if idx := a.ProjectIndex("third"); idx != 2 {
		t.Errorf("ProjectIndex for third = %d, want 2", idx)
	}
}

func TestAllocatorDuplicateVM(t *testing.T) {
	a := NewAllocator("")

	first, err := a.Allocate("proj", "vm-1", "web")
	if err != nil {
		t.Fatalf("first Allocate: %v", err)
	}

	// Allocating the same VM again should return the existing assignment.
	second, err := a.Allocate("proj", "vm-1", "web")
	if err != nil {
		t.Fatalf("second Allocate: %v", err)
	}

	if first.MeshIP != second.MeshIP {
		t.Errorf("duplicate allocation returned different IPs: %q vs %q", first.MeshIP, second.MeshIP)
	}
}

func TestAllocatorRelease(t *testing.T) {
	a := NewAllocator("")

	a.Allocate("proj", "vm-1", "web")

	if err := a.Release("vm-1"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// Should not be found anymore.
	if _, ok := a.GetByVMId("vm-1"); ok {
		t.Error("VM should not be found after release")
	}

	// Releasing again should error.
	if err := a.Release("vm-1"); err == nil {
		t.Error("expected error releasing non-existent VM")
	}
}

func TestAllocatorReleaseNonexistent(t *testing.T) {
	a := NewAllocator("")

	if err := a.Release("nonexistent"); err == nil {
		t.Error("expected error releasing non-existent VM")
	}
}

func TestAllocatorGetByVMId(t *testing.T) {
	a := NewAllocator("")

	a.Allocate("proj", "vm-1", "web")

	assignment, ok := a.GetByVMId("vm-1")
	if !ok {
		t.Fatal("GetByVMId returned false")
	}
	if assignment.MeshIP != "10.90.0.1" {
		t.Errorf("MeshIP = %q, want %q", assignment.MeshIP, "10.90.0.1")
	}

	_, ok = a.GetByVMId("nonexistent")
	if ok {
		t.Error("GetByVMId should return false for nonexistent VM")
	}
}

func TestAllocatorGetByService(t *testing.T) {
	a := NewAllocator("")

	a.Allocate("proj", "vm-1", "web")
	a.Allocate("proj", "vm-2", "db")

	assignment, ok := a.GetByService("proj", "web")
	if !ok {
		t.Fatal("GetByService returned false for web")
	}
	if assignment.VMId != "vm-1" {
		t.Errorf("VMId = %q, want %q", assignment.VMId, "vm-1")
	}

	assignment, ok = a.GetByService("proj", "db")
	if !ok {
		t.Fatal("GetByService returned false for db")
	}
	if assignment.VMId != "vm-2" {
		t.Errorf("VMId = %q, want %q", assignment.VMId, "vm-2")
	}

	// Wrong project.
	_, ok = a.GetByService("other", "web")
	if ok {
		t.Error("GetByService should return false for wrong project")
	}

	// Nonexistent service.
	_, ok = a.GetByService("proj", "cache")
	if ok {
		t.Error("GetByService should return false for nonexistent service")
	}
}

func TestAllocatorListByProject(t *testing.T) {
	a := NewAllocator("")

	a.Allocate("proj", "vm-1", "web")
	a.Allocate("proj", "vm-2", "db")
	a.Allocate("other", "vm-3", "api")

	list := a.ListByProject("proj")
	if len(list) != 2 {
		t.Fatalf("ListByProject returned %d items, want 2", len(list))
	}

	// Check both VMs are present (order is not guaranteed with maps).
	ids := map[string]bool{}
	for _, a := range list {
		ids[a.VMId] = true
	}
	if !ids["vm-1"] || !ids["vm-2"] {
		t.Errorf("ListByProject missing expected VMs: got %v", ids)
	}

	// Empty project.
	list = a.ListByProject("nonexistent")
	if len(list) != 0 {
		t.Errorf("ListByProject for nonexistent returned %d items, want 0", len(list))
	}
}

func TestAllocatorConcurrency(t *testing.T) {
	a := NewAllocator("")

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	// Allocate from multiple goroutines.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			vmId := "vm-" + string(rune('A'+idx))
			_, err := a.Allocate("proj", vmId, "")
			if err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Allocate error: %v", err)
	}

	list := a.ListByProject("proj")
	if len(list) != 50 {
		t.Errorf("expected 50 allocations, got %d", len(list))
	}

	// Verify all IPs are unique.
	ips := map[string]bool{}
	for _, a := range list {
		if ips[a.MeshIP] {
			t.Errorf("duplicate mesh IP: %s", a.MeshIP)
		}
		ips[a.MeshIP] = true
	}
}

func TestAllocatorIPSchemaCorrectness(t *testing.T) {
	a := NewAllocator("")

	// Allocate across multiple projects to verify the IP schema.
	tests := []struct {
		project string
		vmId    string
		wantIP  string
	}{
		{"p0", "v0-0", "10.90.0.1"},
		{"p0", "v0-1", "10.90.0.2"},
		{"p0", "v0-2", "10.90.0.3"},
		{"p1", "v1-0", "10.90.1.1"},
		{"p1", "v1-1", "10.90.1.2"},
		{"p2", "v2-0", "10.90.2.1"},
	}

	for _, tt := range tests {
		assignment, err := a.Allocate(tt.project, tt.vmId, "")
		if err != nil {
			t.Fatalf("Allocate(%s, %s): %v", tt.project, tt.vmId, err)
		}
		if assignment.MeshIP != tt.wantIP {
			t.Errorf("Allocate(%s, %s) = %q, want %q", tt.project, tt.vmId, assignment.MeshIP, tt.wantIP)
		}
	}
}

func TestAllocatorDefaultCIDR(t *testing.T) {
	a := NewAllocator("")
	if a.cidr != MeshCIDR {
		t.Errorf("default CIDR = %q, want %q", a.cidr, MeshCIDR)
	}
}

func TestAllocatorCustomCIDR(t *testing.T) {
	a := NewAllocator("10.99.0.0/16")
	if a.cidr != "10.99.0.0/16" {
		t.Errorf("custom CIDR = %q, want %q", a.cidr, "10.99.0.0/16")
	}
}
