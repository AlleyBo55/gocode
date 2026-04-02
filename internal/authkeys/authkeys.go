package authkeys

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// AuthKey represents a remote access API key.
type AuthKey struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// Store manages auth keys persisted to a JSON file.
type Store struct {
	path string
	keys []AuthKey
}

// NewStore creates a new auth key store at the given path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads keys from disk.
func (s *Store) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.keys = nil
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.keys)
}

// Save writes keys to disk.
func (s *Store) Save() error {
	data, err := json.MarshalIndent(s.keys, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

// Generate creates a new auth key with a random 32-char hex token.
func (s *Store) Generate(name string) (AuthKey, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return AuthKey{}, fmt.Errorf("generating key: %w", err)
	}
	idBytes := make([]byte, 4)
	rand.Read(idBytes)
	ak := AuthKey{
		ID:        hex.EncodeToString(idBytes),
		Key:       hex.EncodeToString(b),
		Name:      name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.keys = append(s.keys, ak)
	return ak, nil
}

// List returns all stored keys.
func (s *Store) List() []AuthKey { return s.keys }

// Delete removes a key by ID.
func (s *Store) Delete(id string) error {
	for i, k := range s.keys {
		if k.ID == id {
			s.keys = append(s.keys[:i], s.keys[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("key not found: %s", id)
}

// Validate checks if a key string matches any stored key.
func (s *Store) Validate(key string) bool {
	for _, k := range s.keys {
		if k.Key == key {
			return true
		}
	}
	return false
}
