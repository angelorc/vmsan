package gateway

import (
	"sync"
	"time"
)

// TimeoutManager manages per-VM timeout timers. When a timer fires, the
// onExpire callback is invoked with the VM ID.
type TimeoutManager struct {
	mu       sync.Mutex
	timers   map[string]*timeoutEntry
	onExpire func(vmId string)
}

type timeoutEntry struct {
	timer     *time.Timer
	timeoutAt time.Time
}

// NewTimeoutManager creates a TimeoutManager with the given expiry callback.
func NewTimeoutManager(onExpire func(vmId string)) *TimeoutManager {
	return &TimeoutManager{
		timers:   make(map[string]*timeoutEntry),
		onExpire: onExpire,
	}
}

// Set starts or resets a timeout timer for the given VM.
func (tm *TimeoutManager) Set(vmId string, timeoutAt time.Time) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Cancel existing timer if present.
	if entry, ok := tm.timers[vmId]; ok {
		entry.timer.Stop()
	}

	dur := time.Until(timeoutAt)
	if dur <= 0 {
		// Already expired — fire immediately in a goroutine.
		go tm.onExpire(vmId)
		delete(tm.timers, vmId)
		return
	}

	timer := time.AfterFunc(dur, func() {
		tm.mu.Lock()
		delete(tm.timers, vmId)
		tm.mu.Unlock()
		tm.onExpire(vmId)
	})

	tm.timers[vmId] = &timeoutEntry{
		timer:     timer,
		timeoutAt: timeoutAt,
	}
}

// Extend updates the timeout for an existing VM. Returns an error if
// the VM has no active timeout.
func (tm *TimeoutManager) Extend(vmId string, newTimeoutAt time.Time) error {
	tm.mu.Lock()
	entry, ok := tm.timers[vmId]
	tm.mu.Unlock()

	if !ok {
		// No existing timer — just set a new one.
		tm.Set(vmId, newTimeoutAt)
		return nil
	}

	_ = entry // existing timer will be replaced by Set
	tm.Set(vmId, newTimeoutAt)
	return nil
}

// Cancel stops and removes the timeout timer for a VM.
func (tm *TimeoutManager) Cancel(vmId string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if entry, ok := tm.timers[vmId]; ok {
		entry.timer.Stop()
		delete(tm.timers, vmId)
	}
}

// CancelAll stops all active timers.
func (tm *TimeoutManager) CancelAll() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for vmId, entry := range tm.timers {
		entry.timer.Stop()
		delete(tm.timers, vmId)
	}
}

// GetTimeoutAt returns the timeout time for a VM if one is set.
func (tm *TimeoutManager) GetTimeoutAt(vmId string) (time.Time, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if entry, ok := tm.timers[vmId]; ok {
		return entry.timeoutAt, true
	}
	return time.Time{}, false
}
