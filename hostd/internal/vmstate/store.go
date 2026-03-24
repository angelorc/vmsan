package vmstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Store provides read/write access to VM state JSON files on disk.
type Store struct {
	dir string
}

// NewStore creates a Store backed by the given directory.
func NewStore(vmsDir string) *Store {
	return &Store{dir: vmsDir}
}

// Load reads a single VM state by ID.
func (s *Store) Load(id string) (*VmState, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var state VmState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// List returns all VM states from the store directory.
func (s *Store) List() ([]*VmState, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var states []*VmState
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}
		var state VmState
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}
		states = append(states, &state)
	}
	return states, nil
}

// Save writes a VM state to disk as pretty-printed JSON.
func (s *Store) Save(state *VmState) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, state.ID+".json"), data, 0600)
}

// Delete removes a VM state file by ID.
func (s *Store) Delete(id string) error {
	return os.Remove(filepath.Join(s.dir, id+".json"))
}

// Update loads a VM state, applies fn, and saves it back.
func (s *Store) Update(id string, fn func(*VmState)) error {
	state, err := s.Load(id)
	if err != nil {
		return err
	}
	fn(state)
	return s.Save(state)
}
