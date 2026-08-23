package uploads

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Gemini keeps an uploaded file for 48h. Caching its URI by content hash is what
// makes a conversation with a clip cheap: the first `ask` uploads, the next ten
// reuse.

// Entry is one cached upload.
type Entry struct {
	Name      string    `json:"name"`
	URI       string    `json:"uri"`
	MimeType  string    `json:"mime_type"`
	Bytes     int64     `json:"bytes"`
	Source    string    `json:"source"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Store is the on-disk cache (sha256 → Entry).
type Store struct {
	path    string
	Entries map[string]Entry `json:"entries"`
}

// Open loads (or starts) the store at dir/uploads.json.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "uploads.json"), Entries: map[string]Entry{}}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("uploads.Open: %w", err)
	}
	if err := json.Unmarshal(data, s); err != nil {
		// a corrupt cache is not worth failing a run over
		s.Entries = map[string]Entry{}
	}
	if s.Entries == nil {
		s.Entries = map[string]Entry{}
	}
	return s, nil
}

// Get returns a live entry for a hash, dropping anything expired or near expiry.
func (s *Store) Get(hash string) (Entry, bool) {
	e, ok := s.Entries[hash]
	if !ok {
		return Entry{}, false
	}
	if !e.ExpiresAt.IsZero() && time.Until(e.ExpiresAt) < 5*time.Minute {
		delete(s.Entries, hash)
		return Entry{}, false
	}
	return e, true
}

// Put records an entry and persists the store.
func (s *Store) Put(hash string, e Entry) error {
	s.Entries[hash] = e
	return s.save()
}

func (s *Store) save() error {
	now := time.Now()
	for k, e := range s.Entries {
		if !e.ExpiresAt.IsZero() && e.ExpiresAt.Before(now) {
			delete(s.Entries, k)
		}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("uploads.save: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("uploads.save: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("uploads.save: %w", err)
	}
	return nil
}

// HashFile returns the sha256 of a file's contents.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("uploads.HashFile: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("uploads.HashFile: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
