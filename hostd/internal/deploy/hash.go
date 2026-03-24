package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// hashStore manages deploy hash persistence in ~/.vmsan/deploy-hashes.json.
type hashStore struct {
	path string
	mu   sync.Mutex
}

// NewHashStore creates a hash store at the given base directory.
func NewHashStore(baseDir string) *hashStore {
	return &hashStore{
		path: filepath.Join(baseDir, "deploy-hashes.json"),
	}
}

// Get returns the deploy hash for a VM, or empty string if not found.
func (h *hashStore) Get(vmID string) string {
	hashes := h.load()
	return hashes[vmID]
}

// Set stores the deploy hash for a VM.
func (h *hashStore) Set(vmID, hash string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	hashes := h.load()
	hashes[vmID] = hash
	return h.save(hashes)
}

// Remove deletes the deploy hash for a VM.
func (h *hashStore) Remove(vmID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	hashes := h.load()
	delete(hashes, vmID)
	return h.save(hashes)
}

func (h *hashStore) load() map[string]string {
	data, err := os.ReadFile(h.path)
	if err != nil {
		return make(map[string]string)
	}
	var hashes map[string]string
	if err := json.Unmarshal(data, &hashes); err != nil {
		return make(map[string]string)
	}
	return hashes
}

func (h *hashStore) save(hashes map[string]string) error {
	dir := filepath.Dir(h.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(hashes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(h.path, data, 0600)
}
