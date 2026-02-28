package cmdstore

import (
	"crypto/rand"
	"encoding/hex"
	"os/exec"
	"sync"
)

var (
	mu    sync.RWMutex
	store = make(map[string]*exec.Cmd)
)

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Store registers a running command and returns its ID.
func Store(cmd *exec.Cmd) string {
	id := generateID()
	mu.Lock()
	store[id] = cmd
	mu.Unlock()
	return id
}

// Get returns the command for the given ID, or nil.
func Get(id string) *exec.Cmd {
	mu.RLock()
	defer mu.RUnlock()
	return store[id]
}

// Remove deletes a command from the store.
func Remove(id string) {
	mu.Lock()
	delete(store, id)
	mu.Unlock()
}
