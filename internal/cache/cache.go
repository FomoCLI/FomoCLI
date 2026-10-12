// Package cache is the on-disk response store behind fomo's offline mode.
//
// Every successful API response is written to $XDG_CACHE_HOME/fomo/. That buys
// two things: repeated calls inside the TTL cost zero credits, and with
// --offline (or when the network is down) every command can still render the
// last data it saw - no internet required.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry is one cached API response.
type Entry struct {
	Path      string          `json:"path"`
	FetchedAt time.Time       `json:"fetched_at"`
	Body      json.RawMessage `json:"body"`
}

// Age is how long ago the entry was fetched.
func (e *Entry) Age() time.Duration { return time.Since(e.FetchedAt) }

// Store is a directory of cached entries.
type Store struct{ Dir string }

// Default returns the store in the user's cache dir, honoring FOMO_CACHE_DIR.
func Default() (*Store, error) {
	if d := os.Getenv("FOMO_CACHE_DIR"); d != "" {
		return &Store{Dir: d}, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return &Store{Dir: filepath.Join(base, "fomo")}, nil
}

// Key derives a stable file name from a request path (including query).
func Key(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:12])
}

func (s *Store) file(path string) string {
	return filepath.Join(s.Dir, Key(path)+".json")
}

// Get returns the cached entry for a path, or nil.
func (s *Store) Get(path string) (*Entry, error) {
	b, err := os.ReadFile(s.file(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, nil // corrupt entry: treat as a miss
	}
	return &e, nil
}

// Put stores a response body for a path.
func (s *Store) Put(path string, body []byte) error {
	if !json.Valid(body) {
		return nil
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(Entry{Path: path, FetchedAt: time.Now().UTC(), Body: body})
	if err != nil {
		return err
	}
	tmp := s.file(path) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.file(path))
}

// Info summarizes one cached entry without its body.
type Info struct {
	Path      string
	FetchedAt time.Time
	Size      int64
}

// List returns all entries, newest first.
func (s *Store) List() ([]Info, error) {
	ents, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, de := range ents {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		full := filepath.Join(s.Dir, de.Name())
		b, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		var e struct {
			Path      string    `json:"path"`
			FetchedAt time.Time `json:"fetched_at"`
		}
		if json.Unmarshal(b, &e) != nil {
			continue
		}
		out = append(out, Info{Path: e.Path, FetchedAt: e.FetchedAt, Size: int64(len(b))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FetchedAt.After(out[j].FetchedAt) })
	return out, nil
}

// Clear deletes entries. With olderThan > 0 only entries older than that go.
// It returns how many were removed.
func (s *Store) Clear(olderThan time.Duration) (int, error) {
	ents, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, de := range ents {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		full := filepath.Join(s.Dir, de.Name())
		if olderThan > 0 {
			fi, err := de.Info()
			if err != nil || time.Since(fi.ModTime()) < olderThan {
				continue
			}
		}
		if os.Remove(full) == nil {
			n++
		}
	}
	return n, nil
}
