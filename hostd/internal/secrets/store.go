package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// encryptedValue is the on-disk format for a single encrypted secret.
type encryptedValue struct {
	IV   string `json:"iv"`
	Tag  string `json:"tag"`
	Data string `json:"data"`
}

// Store provides encrypted per-project secret storage.
// Keys are stored in ~/.vmsan/keys/{project}.key
// Encrypted secrets are stored in ~/.vmsan/secrets/{project}.enc
type Store struct {
	keysDir    string
	secretsDir string
}

// NewStore creates a secrets store backed by the given directories.
func NewStore(baseDir string) *Store {
	return &Store{
		keysDir:    filepath.Join(baseDir, "keys"),
		secretsDir: filepath.Join(baseDir, "secrets"),
	}
}

// Set encrypts and stores a secret for the given project.
func (s *Store) Set(project, key, value string) error {
	encKey, err := s.getOrCreateKey(project)
	if err != nil {
		return fmt.Errorf("get encryption key: %w", err)
	}

	encrypted, err := encrypt(encKey, []byte(value))
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	secrets, err := s.loadSecrets(project)
	if err != nil {
		secrets = make(map[string]encryptedValue)
	}

	secrets[key] = *encrypted
	return s.saveSecrets(project, secrets)
}

// Get decrypts and returns a secret. Returns empty string and nil error if not found.
func (s *Store) Get(project, key string) (string, error) {
	encKey, err := s.loadKey(project)
	if err != nil {
		return "", nil // no key = no secrets
	}

	secrets, err := s.loadSecrets(project)
	if err != nil {
		return "", nil
	}

	ev, ok := secrets[key]
	if !ok {
		return "", nil
	}

	plaintext, err := decrypt(encKey, &ev)
	if err != nil {
		return "", fmt.Errorf("decrypt %q: %w", key, err)
	}

	return string(plaintext), nil
}

// List returns all secret key names for a project.
func (s *Store) List(project string) ([]string, error) {
	secrets, err := s.loadSecrets(project)
	if err != nil {
		return nil, nil
	}

	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	return keys, nil
}

// Unset removes a secret. Returns true if the key existed.
func (s *Store) Unset(project, key string) (bool, error) {
	secrets, err := s.loadSecrets(project)
	if err != nil {
		return false, nil
	}

	if _, ok := secrets[key]; !ok {
		return false, nil
	}

	delete(secrets, key)
	return true, s.saveSecrets(project, secrets)
}

// GetAll decrypts and returns all secrets for a project as a map.
func (s *Store) GetAll(project string) (map[string]string, error) {
	encKey, err := s.loadKey(project)
	if err != nil {
		return nil, nil
	}

	secrets, err := s.loadSecrets(project)
	if err != nil {
		return nil, nil
	}

	result := make(map[string]string, len(secrets))
	for k, ev := range secrets {
		plaintext, err := decrypt(encKey, &ev)
		if err != nil {
			continue
		}
		result[k] = string(plaintext)
	}
	return result, nil
}

// --- key management ---

func (s *Store) getOrCreateKey(project string) ([]byte, error) {
	key, err := s.loadKey(project)
	if err == nil {
		return key, nil
	}

	// Generate new 32-byte key
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(s.keysDir, 0700); err != nil {
		return nil, err
	}

	keyHex := hex.EncodeToString(key)
	path := filepath.Join(s.keysDir, project+".key")
	if err := os.WriteFile(path, []byte(keyHex), 0600); err != nil {
		return nil, err
	}

	return key, nil
}

func (s *Store) loadKey(project string) ([]byte, error) {
	path := filepath.Join(s.keysDir, project+".key")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(string(data))
}

// --- secrets file ---

func (s *Store) loadSecrets(project string) (map[string]encryptedValue, error) {
	path := filepath.Join(s.secretsDir, project+".enc")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var secrets map[string]encryptedValue
	if err := json.Unmarshal(data, &secrets); err != nil {
		return nil, err
	}
	return secrets, nil
}

func (s *Store) saveSecrets(project string, secrets map[string]encryptedValue) error {
	if err := os.MkdirAll(s.secretsDir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.secretsDir, project+".enc"), data, 0600)
}

// --- AES-256-GCM ---

func encrypt(key, plaintext []byte) (*encryptedValue, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	// Seal appends the ciphertext + tag
	sealed := gcm.Seal(nil, iv, plaintext, nil)

	// Split ciphertext and tag (last 16 bytes)
	tagSize := gcm.Overhead()
	ciphertext := sealed[:len(sealed)-tagSize]
	tag := sealed[len(sealed)-tagSize:]

	return &encryptedValue{
		IV:   hex.EncodeToString(iv),
		Tag:  hex.EncodeToString(tag),
		Data: hex.EncodeToString(ciphertext),
	}, nil
}

func decrypt(key []byte, ev *encryptedValue) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	iv, err := hex.DecodeString(ev.IV)
	if err != nil {
		return nil, fmt.Errorf("decode iv: %w", err)
	}
	ciphertext, err := hex.DecodeString(ev.Data)
	if err != nil {
		return nil, fmt.Errorf("decode data: %w", err)
	}
	tag, err := hex.DecodeString(ev.Tag)
	if err != nil {
		return nil, fmt.Errorf("decode tag: %w", err)
	}

	// Reconstruct sealed = ciphertext + tag
	sealed := append(ciphertext, tag...)

	return gcm.Open(nil, iv, sealed, nil)
}
