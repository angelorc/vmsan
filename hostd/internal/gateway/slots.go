package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// SlotAllocator manages per-VM network slot assignments.
// Slots determine IP addresses, port numbers, and device names.
type SlotAllocator struct {
	mu       sync.Mutex
	used     map[int]string // slot -> vmId
	reverse  map[string]int // vmId -> slot
	max      int
	filePath string // persistence path
}

// NewSlotAllocator creates a slot allocator with optional persistence.
func NewSlotAllocator(maxSlots int, filePath string) *SlotAllocator {
	a := &SlotAllocator{
		used:     make(map[int]string),
		reverse:  make(map[string]int),
		max:      maxSlots,
		filePath: filePath,
	}
	// Load persisted state on startup
	if filePath != "" {
		a.load()
	}
	return a
}

// Allocate assigns the next free slot to a VM. Returns the slot number.
func (a *SlotAllocator) Allocate(vmId string) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if already allocated
	if slot, ok := a.reverse[vmId]; ok {
		return slot, nil
	}

	// Find first free slot
	for slot := 0; slot <= a.max; slot++ {
		if _, used := a.used[slot]; !used {
			a.used[slot] = vmId
			a.reverse[vmId] = slot
			a.persist()
			return slot, nil
		}
	}

	return -1, fmt.Errorf("no free network slots (max %d)", a.max)
}

// Release frees a slot by VM ID.
func (a *SlotAllocator) Release(vmId string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if slot, ok := a.reverse[vmId]; ok {
		delete(a.used, slot)
		delete(a.reverse, vmId)
		a.persist()
	}
}

// GetSlot returns the slot for a VM ID, or -1 if not found.
func (a *SlotAllocator) GetSlot(vmId string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if slot, ok := a.reverse[vmId]; ok {
		return slot
	}
	return -1
}

// IsUsed checks if a slot is in use.
func (a *SlotAllocator) IsUsed(slot int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, used := a.used[slot]
	return used
}

// Count returns the number of allocated slots.
func (a *SlotAllocator) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.used)
}

// persist writes the slot map to disk for crash recovery.
func (a *SlotAllocator) persist() {
	if a.filePath == "" {
		return
	}
	data, err := json.Marshal(a.used)
	if err != nil {
		return
	}
	_ = os.WriteFile(a.filePath, data, 0644)
}

// load reads the slot map from disk.
func (a *SlotAllocator) load() {
	data, err := os.ReadFile(a.filePath)
	if err != nil {
		return
	}
	var slots map[int]string
	if err := json.Unmarshal(data, &slots); err != nil {
		return
	}
	a.used = slots
	a.reverse = make(map[string]int, len(slots))
	for slot, vmId := range slots {
		a.reverse[vmId] = slot
	}
}
