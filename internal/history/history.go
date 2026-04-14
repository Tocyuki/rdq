// Package history persists executed SQL statements as a per-line JSON log so
// the TUI can recall them via an incremental search picker.
//
// The on-disk format is JSON Lines (one entry per line) at
// ~/.rdq/history.jsonl, overridable via the RDQ_HISTORY_FILE env var. Append
// is O(1); Load streams the whole file and filters in memory.
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry is one recorded statement execution.
type Entry struct {
	Profile    string    `json:"profile"`
	Database   string    `json:"database"`
	SQL        string    `json:"sql"`
	At         time.Time `json:"at"`
	Ok         bool      `json:"ok"`
	DurationMS int64     `json:"duration_ms"`
	ErrorMsg   string    `json:"error,omitempty"`
	// Favorite marks an entry as starred. Favorites survive across runs
	// because the on-disk JSONL is rewritten by SetFavorite.
	Favorite bool `json:"favorite,omitempty"`
}

// Store is a thin wrapper over the on-disk JSONL file.
type Store struct {
	path string
}

// New resolves the history file path (honoring RDQ_HISTORY_FILE) and returns
// a ready-to-use Store. The file itself is created lazily on Append.
func New() (*Store, error) {
	path, err := resolvePath()
	if err != nil {
		return nil, err
	}
	return &Store{path: path}, nil
}

// Path returns the resolved history file location.
func (s *Store) Path() string { return s.path }

// Append writes an entry as one JSON line. The file and parent directory are
// created on demand.
func (s *Store) Append(e Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create history directory: %w", err)
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal history entry: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write history entry: %w", err)
	}
	return nil
}

// Load returns entries matching the given profile and database, ordered with
// the most recent first. A missing file is not an error — it returns an empty
// slice. Lines that fail to parse are skipped so that a single bad line cannot
// break recall.
func (s *Store) Load(profile, database string) ([]Entry, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	var out []Entry
	scanner := bufio.NewScanner(f)
	// Allow long SQL statements; default 64 KiB per line is too small for
	// users pasting in real workloads.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if e.Profile != profile || e.Database != database {
			continue
		}
		out = append(out, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan history file: %w", err)
	}
	reverse(out)
	return out, nil
}

// SetFavorite toggles the Favorite flag on the entry whose At timestamp
// matches the given time (within nanosecond precision). The whole JSONL
// file is rewritten atomically (tempfile + rename) because the format is
// append-only and there is no random-access slot for a single field.
//
// Entries from other profiles / databases are preserved untouched. If no
// matching entry is found the function is a no-op (returns nil).
func (s *Store) SetFavorite(at time.Time, favorite bool) error {
	entries, err := s.loadAll()
	if err != nil {
		return err
	}
	matched := false
	for i := range entries {
		if entries[i].At.Equal(at) {
			entries[i].Favorite = favorite
			matched = true
		}
	}
	if !matched {
		return nil
	}
	return s.rewriteAll(entries)
}

// loadAll reads every entry from the history file regardless of profile /
// database. Used internally by SetFavorite to round-trip the whole log.
func (s *Store) loadAll() ([]Entry, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	var out []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan history file: %w", err)
	}
	return out, nil
}

// rewriteAll writes the supplied slice back to the history file atomically
// via a tempfile rename. Used by SetFavorite to commit a Favorite toggle.
func (s *Store) rewriteAll(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create history directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".history-*.jsonl")
	if err != nil {
		return fmt.Errorf("create temp history file: %w", err)
	}
	tmpName := tmp.Name()
	w := bufio.NewWriter(tmp)
	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return fmt.Errorf("marshal history entry: %w", err)
		}
		if _, err := w.Write(append(data, '\n')); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return fmt.Errorf("write history entry: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("flush history file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp history file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp history file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename history file: %w", err)
	}
	return nil
}

func reverse(s []Entry) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func resolvePath() (string, error) {
	if override := os.Getenv("RDQ_HISTORY_FILE"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".rdq", "history.jsonl"), nil
}
