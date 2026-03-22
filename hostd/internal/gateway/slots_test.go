package gateway

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAllocateSequential(t *testing.T) {
	a := NewSlotAllocator(254, "")

	slot0, err := a.Allocate("vm-aaa")
	if err != nil {
		t.Fatalf("allocate vm-aaa: %v", err)
	}
	if slot0 != 0 {
		t.Errorf("expected slot 0, got %d", slot0)
	}

	slot1, err := a.Allocate("vm-bbb")
	if err != nil {
		t.Fatalf("allocate vm-bbb: %v", err)
	}
	if slot1 != 1 {
		t.Errorf("expected slot 1, got %d", slot1)
	}

	slot2, err := a.Allocate("vm-ccc")
	if err != nil {
		t.Fatalf("allocate vm-ccc: %v", err)
	}
	if slot2 != 2 {
		t.Errorf("expected slot 2, got %d", slot2)
	}
}

func TestAllocateIdempotent(t *testing.T) {
	a := NewSlotAllocator(254, "")

	slot1, err := a.Allocate("vm-abc")
	if err != nil {
		t.Fatalf("first allocate: %v", err)
	}

	slot2, err := a.Allocate("vm-abc")
	if err != nil {
		t.Fatalf("second allocate: %v", err)
	}

	if slot1 != slot2 {
		t.Errorf("expected same slot, got %d and %d", slot1, slot2)
	}

	if a.Count() != 1 {
		t.Errorf("expected count 1, got %d", a.Count())
	}
}

func TestReleaseFreesSlot(t *testing.T) {
	a := NewSlotAllocator(254, "")

	_, _ = a.Allocate("vm-1")
	_, _ = a.Allocate("vm-2")
	_, _ = a.Allocate("vm-3")

	// Release slot 1 (vm-2)
	a.Release("vm-2")

	if a.IsUsed(1) {
		t.Error("slot 1 should be free after release")
	}
	if a.GetSlot("vm-2") != -1 {
		t.Error("vm-2 should not have a slot after release")
	}

	// Next allocation should reuse slot 1
	slot, err := a.Allocate("vm-4")
	if err != nil {
		t.Fatalf("allocate vm-4: %v", err)
	}
	if slot != 1 {
		t.Errorf("expected slot 1 to be reused, got %d", slot)
	}
}

func TestNoFreeSlots(t *testing.T) {
	a := NewSlotAllocator(2, "") // slots 0, 1, 2

	_, _ = a.Allocate("vm-0")
	_, _ = a.Allocate("vm-1")
	_, _ = a.Allocate("vm-2")

	_, err := a.Allocate("vm-3")
	if err == nil {
		t.Error("expected error when no free slots")
	}
}

func TestConcurrentAllocation(t *testing.T) {
	a := NewSlotAllocator(254, "")
	n := 100

	var wg sync.WaitGroup
	results := make([]int, n)
	errors := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			slot, err := a.Allocate(fmt.Sprintf("vm-%d", idx))
			results[idx] = slot
			errors[idx] = err
		}(i)
	}
	wg.Wait()

	// Check no errors
	for i, err := range errors {
		if err != nil {
			t.Errorf("vm-%d allocation failed: %v", i, err)
		}
	}

	// Check all slots are unique
	seen := make(map[int]bool)
	for i, slot := range results {
		if seen[slot] {
			t.Errorf("duplicate slot %d for vm-%d", slot, i)
		}
		seen[slot] = true
	}

	if a.Count() != n {
		t.Errorf("expected count %d, got %d", n, a.Count())
	}
}

func TestPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "slots.json")

	// Create allocator and allocate some slots
	a1 := NewSlotAllocator(254, filePath)
	_, _ = a1.Allocate("vm-aaa")
	_, _ = a1.Allocate("vm-bbb")

	// Verify file was written
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("persistence file not created: %v", err)
	}

	// Create new allocator from the same file
	a2 := NewSlotAllocator(254, filePath)

	if a2.GetSlot("vm-aaa") != 0 {
		t.Errorf("expected vm-aaa at slot 0 after reload, got %d", a2.GetSlot("vm-aaa"))
	}
	if a2.GetSlot("vm-bbb") != 1 {
		t.Errorf("expected vm-bbb at slot 1 after reload, got %d", a2.GetSlot("vm-bbb"))
	}
	if a2.Count() != 2 {
		t.Errorf("expected count 2 after reload, got %d", a2.Count())
	}
}
