package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretsStore_SetGet(t *testing.T) {
	store := NewStore(t.TempDir())

	if err := store.Set("proj", "KEY", "secret-value"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, err := store.Get("proj", "KEY")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "secret-value" {
		t.Errorf("Get() = %q, want %q", got, "secret-value")
	}
}

func TestSecretsStore_GetMissing(t *testing.T) {
	store := NewStore(t.TempDir())

	got, err := store.Get("proj", "NOKEY")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "" {
		t.Errorf("Get() = %q, want empty string", got)
	}
}

func TestSecretsStore_List(t *testing.T) {
	store := NewStore(t.TempDir())

	for _, k := range []string{"A", "B", "C"} {
		if err := store.Set("proj", k, "val-"+k); err != nil {
			t.Fatalf("Set(%q) error: %v", k, err)
		}
	}

	keys, err := store.List("proj")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("List() returned %d keys, want 3", len(keys))
	}

	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	for _, want := range []string{"A", "B", "C"} {
		if !keySet[want] {
			t.Errorf("List() missing key %q", want)
		}
	}
}

func TestSecretsStore_Unset(t *testing.T) {
	store := NewStore(t.TempDir())

	if err := store.Set("proj", "KEY", "value"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	existed, err := store.Unset("proj", "KEY")
	if err != nil {
		t.Fatalf("Unset() error: %v", err)
	}
	if !existed {
		t.Error("Unset() = false, want true (key existed)")
	}

	got, err := store.Get("proj", "KEY")
	if err != nil {
		t.Fatalf("Get() after Unset error: %v", err)
	}
	if got != "" {
		t.Errorf("Get() after Unset = %q, want empty string", got)
	}

	existed, err = store.Unset("proj", "NONEXISTENT")
	if err != nil {
		t.Fatalf("Unset(NONEXISTENT) error: %v", err)
	}
	if existed {
		t.Error("Unset(NONEXISTENT) = true, want false")
	}
}

func TestSecretsStore_GetAll(t *testing.T) {
	store := NewStore(t.TempDir())

	secrets := map[string]string{
		"DB_HOST":     "localhost",
		"DB_PASSWORD": "s3cret",
		"API_KEY":     "abc123",
	}
	for k, v := range secrets {
		if err := store.Set("proj", k, v); err != nil {
			t.Fatalf("Set(%q) error: %v", k, err)
		}
	}

	got, err := store.GetAll("proj")
	if err != nil {
		t.Fatalf("GetAll() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("GetAll() returned %d entries, want 3", len(got))
	}

	for k, want := range secrets {
		if got[k] != want {
			t.Errorf("GetAll()[%q] = %q, want %q", k, got[k], want)
		}
	}
}

func TestSecretsStore_ProjectIsolation(t *testing.T) {
	store := NewStore(t.TempDir())

	if err := store.Set("proj1", "KEY", "value-from-proj1"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, err := store.Get("proj2", "KEY")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "" {
		t.Errorf("Get(proj2, KEY) = %q, want empty string (project isolation)", got)
	}
}

func TestSecretsStore_EncryptedOnDisk(t *testing.T) {
	baseDir := t.TempDir()
	store := NewStore(baseDir)

	plaintext := "my-super-secret-value"
	if err := store.Set("proj", "SECRET", plaintext); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	encFile := filepath.Join(baseDir, "secrets", "proj.enc")
	data, err := os.ReadFile(encFile)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", encFile, err)
	}

	raw := string(data)
	if strings.Contains(raw, plaintext) {
		t.Error("encrypted file contains plaintext secret value; expected it to be encrypted")
	}

	// Verify the file is valid JSON with iv/tag/data fields
	if !strings.Contains(raw, "iv") {
		t.Error("encrypted file missing 'iv' field")
	}
	if !strings.Contains(raw, "tag") {
		t.Error("encrypted file missing 'tag' field")
	}
	if !strings.Contains(raw, "data") {
		t.Error("encrypted file missing 'data' field")
	}
}
