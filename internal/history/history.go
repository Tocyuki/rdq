// Package history persists executed SQL statements as a per-line JSON log so
// the TUI can recall them via an incremental search picker.
//
// The on-disk format is JSON Lines (one entry per line) at
// ~/.rdq/history.jsonl, overridable via the RDQ_HISTORY_FILE env var. Append
// deduplicates: re-running a statement that already exists for the same
// (profile, database) moves it to the tail with fresh metadata, preserving
// the user's favourite star.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tocyuki/rdq/internal/fsutil"
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

// Append writes an entry to the history file, deduplicating against any
// prior entry with the same (Profile, Database, TrimSpace(SQL)). The stale
// entry is removed and the new one takes its slot at the end of the file
// so re-runs of an identical statement do not clutter the picker.
//
// `At`, `Ok`, `DurationMS`, and `ErrorMsg` always reflect the current run.
// `Favorite` is inherited from the existing entry so the user's star
// survives re-execution even if the caller passes `e.Favorite=false`.
//
// The implementation is O(n) on disk because the JSONL log is rewritten in
// place (via the same tempfile+rename path SetFavorite uses). Histories
// are typically small (< a few thousand lines) so the cost is negligible,
// and the tradeoff buys simple in-order recall by Load. The whole
// read-modify-write runs under an inter-process lock so TUI sessions and
// `rdq exec` appending at the same time never drop each other's entries.
func (s *Store) Append(e Entry) error {
	return fsutil.WithLock(s.path, func() error {
		lines, err := s.readLines()
		if err != nil {
			return err
		}
		needle := strings.TrimSpace(e.SQL)
		kept := make([]line, 0, len(lines)+1)
		for _, l := range lines {
			if old := l.entry; old != nil &&
				old.Profile == e.Profile &&
				old.Database == e.Database &&
				strings.TrimSpace(old.SQL) == needle {
				if old.Favorite {
					e.Favorite = true
				}
				continue
			}
			kept = append(kept, l)
		}
		kept = append(kept, line{entry: &e})
		return s.writeLines(kept)
	})
}

// Record appends the outcome of one statement execution. It is the single
// place the TUI and CLI build history entries from, so all surfaces
// record identical fields. Ephemeral mode (empty profile) is a no-op: direct
// credentials runs leave no on-disk trace.
func (s *Store) Record(profile, database, sql string, execErr error, elapsed time.Duration) error {
	if s == nil || profile == "" {
		return nil
	}
	entry := Entry{
		Profile:    profile,
		Database:   database,
		SQL:        sql,
		At:         time.Now(),
		Ok:         execErr == nil,
		DurationMS: elapsed.Milliseconds(),
	}
	if execErr != nil {
		entry.ErrorMsg = execErr.Error()
	}
	return s.Append(entry)
}

// Load returns entries matching the given profile and database, ordered with
// the most recent first. A missing file is not an error — it returns an empty
// slice. Lines that fail to parse are skipped so that a single bad line cannot
// break recall.
func (s *Store) Load(profile, database string) ([]Entry, error) {
	lines, err := s.readLines()
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, l := range lines {
		if l.entry != nil && l.entry.Profile == profile && l.entry.Database == database {
			out = append(out, *l.entry)
		}
	}
	reverse(out)
	return out, nil
}

// SetFavorite toggles the Favorite flag on the entry whose At timestamp
// matches the given time (within nanosecond precision). The whole JSONL
// file is rewritten atomically (tempfile + rename) under the history lock
// because the format is append-only and there is no random-access slot
// for a single field.
//
// Entries from other profiles / databases are preserved untouched. If no
// matching entry is found the function is a no-op (returns nil).
func (s *Store) SetFavorite(at time.Time, favorite bool) error {
	return fsutil.WithLock(s.path, func() error {
		lines, err := s.readLines()
		if err != nil {
			return err
		}
		matched := false
		for _, l := range lines {
			if l.entry != nil && l.entry.At.Equal(at) {
				l.entry.Favorite = favorite
				matched = true
			}
		}
		if !matched {
			return nil
		}
		return s.writeLines(lines)
	})
}

// line is one physical line of the history file. entry is nil when the
// line did not parse; raw then holds it verbatim so a rewrite preserves it
// instead of silently deleting data (e.g. a line written by a newer rdq
// with a field type this version does not understand).
type line struct {
	entry *Entry
	raw   []byte
}

// readLines reads every line of the history file. bufio.Reader (rather
// than a Scanner) is used so there is no per-line size cap: one huge
// pasted statement must not make the whole history unreadable.
func (s *Store) readLines() ([]line, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	var out []line
	r := bufio.NewReader(f)
	for {
		raw, readErr := r.ReadBytes('\n')
		raw = bytes.TrimRight(raw, "\r\n")
		if len(bytes.TrimSpace(raw)) > 0 {
			var e Entry
			if err := json.Unmarshal(raw, &e); err != nil {
				out = append(out, line{raw: raw})
			} else {
				out = append(out, line{entry: &e})
			}
		}
		if readErr == io.EOF {
			return out, nil
		}
		if readErr != nil {
			return nil, fmt.Errorf("read history file: %w", readErr)
		}
	}
}

// writeLines serializes lines and atomically replaces the history file.
// Callers must hold the history lock.
func (s *Store) writeLines(lines []line) error {
	var buf bytes.Buffer
	for _, l := range lines {
		data := l.raw
		if l.entry != nil {
			var err error
			if data, err = json.Marshal(l.entry); err != nil {
				return fmt.Errorf("marshal history entry: %w", err)
			}
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	return fsutil.WriteFileAtomic(s.path, buf.Bytes(), 0o600)
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
