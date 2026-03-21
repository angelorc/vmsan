package gateway

import (
	"sync"
	"testing"
)

func TestStartVM(t *testing.T) {
	m := NewManager()

	state, err := m.StartVM("vm-1", 1, "deny-all")
	if err != nil {
		t.Fatalf("StartVM: unexpected error: %v", err)
	}
	if state.VMId != "vm-1" {
		t.Errorf("VMId = %q, want %q", state.VMId, "vm-1")
	}
	if state.Slot != 1 {
		t.Errorf("Slot = %d, want %d", state.Slot, 1)
	}
	if state.Policy != "deny-all" {
		t.Errorf("Policy = %q, want %q", state.Policy, "deny-all")
	}
	if state.DNSPort == 0 || state.SNIPort == 0 || state.HTTPPort == 0 {
		t.Error("ports should be non-zero")
	}
}

func TestStartVM_Idempotent(t *testing.T) {
	m := NewManager()

	first, err := m.StartVM("vm-1", 1, "deny-all")
	if err != nil {
		t.Fatalf("first StartVM: %v", err)
	}

	second, err := m.StartVM("vm-1", 2, "allow")
	if err != nil {
		t.Fatalf("second StartVM: %v", err)
	}

	if first != second {
		t.Error("second StartVM should return the same pointer")
	}
	if second.Slot != 1 {
		t.Errorf("slot should remain %d from first call, got %d", 1, second.Slot)
	}
}

func TestStopVM(t *testing.T) {
	m := NewManager()

	m.StartVM("vm-1", 1, "deny-all")

	if err := m.StopVM("vm-1"); err != nil {
		t.Fatalf("StopVM: unexpected error: %v", err)
	}

	if _, ok := m.GetVM("vm-1"); ok {
		t.Error("VM should be removed after StopVM")
	}
}

func TestStopVM_NotFound(t *testing.T) {
	m := NewManager()

	if err := m.StopVM("nonexistent"); err == nil {
		t.Error("StopVM on nonexistent VM should return error")
	}
}

func TestUpdatePolicy(t *testing.T) {
	m := NewManager()

	m.StartVM("vm-1", 1, "deny-all")

	if err := m.UpdatePolicy("vm-1", "allow"); err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}

	state, ok := m.GetVM("vm-1")
	if !ok {
		t.Fatal("VM should exist after UpdatePolicy")
	}
	if state.Policy != "allow" {
		t.Errorf("Policy = %q, want %q", state.Policy, "allow")
	}
}

func TestUpdatePolicy_NotFound(t *testing.T) {
	m := NewManager()

	if err := m.UpdatePolicy("nonexistent", "allow"); err == nil {
		t.Error("UpdatePolicy on nonexistent VM should return error")
	}
}

func TestGetVM(t *testing.T) {
	m := NewManager()

	if _, ok := m.GetVM("vm-1"); ok {
		t.Error("GetVM should return false for nonexistent VM")
	}

	m.StartVM("vm-1", 1, "deny-all")

	state, ok := m.GetVM("vm-1")
	if !ok {
		t.Fatal("GetVM should return true after StartVM")
	}
	if state.VMId != "vm-1" {
		t.Errorf("VMId = %q, want %q", state.VMId, "vm-1")
	}
}

func TestListVMs(t *testing.T) {
	m := NewManager()

	if vms := m.ListVMs(); len(vms) != 0 {
		t.Errorf("ListVMs on empty manager: got %d, want 0", len(vms))
	}

	m.StartVM("vm-1", 1, "deny-all")
	m.StartVM("vm-2", 2, "allow")

	vms := m.ListVMs()
	if len(vms) != 2 {
		t.Errorf("ListVMs: got %d, want 2", len(vms))
	}

	// Verify both VMs are present.
	ids := make(map[string]bool)
	for _, vm := range vms {
		ids[vm.VMId] = true
	}
	if !ids["vm-1"] || !ids["vm-2"] {
		t.Errorf("ListVMs missing expected VMs: %v", ids)
	}
}

func TestStopAll(t *testing.T) {
	m := NewManager()

	m.StartVM("vm-1", 1, "deny-all")
	m.StartVM("vm-2", 2, "allow")
	m.StartVM("vm-3", 3, "deny-all")

	m.StopAll()

	if vms := m.ListVMs(); len(vms) != 0 {
		t.Errorf("ListVMs after StopAll: got %d, want 0", len(vms))
	}

	if _, ok := m.GetVM("vm-1"); ok {
		t.Error("vm-1 should not exist after StopAll")
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup

	// Concurrently start VMs.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			vmId := "vm-" + itoa(id)
			m.StartVM(vmId, id, "deny-all")
		}(i)
	}
	wg.Wait()

	if vms := m.ListVMs(); len(vms) != 100 {
		t.Errorf("after concurrent starts: got %d VMs, want 100", len(vms))
	}

	// Concurrently stop VMs.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			vmId := "vm-" + itoa(id)
			m.StopVM(vmId)
		}(i)
	}
	wg.Wait()

	if vms := m.ListVMs(); len(vms) != 0 {
		t.Errorf("after concurrent stops: got %d VMs, want 0", len(vms))
	}
}

// itoa is a tiny helper to avoid importing strconv in tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
