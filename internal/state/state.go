// Package state persists per-profile selection state for rdq.
//
// The state file lives at ~/.rdq/state.json (overridable via the RDQ_STATE_FILE
// env var for tests). Each AWS profile gets its own entry so that switching
// profiles does not leak cluster/secret/database history between accounts.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tocyuki/rdq/internal/fsutil"
)

// historyLimit caps the database name history per profile. The most recent
// entry is at index 0; older entries are dropped past the limit.
const historyLimit = 10

type State struct {
	Profiles map[string]ProfileState `json:"profiles"`

	path string
}

type ProfileState struct {
	Cluster         string   `json:"cluster,omitempty"`
	Secret          string   `json:"secret,omitempty"`
	Database        string   `json:"database,omitempty"`
	BedrockModel    string   `json:"bedrock_model,omitempty"`
	BedrockLanguage string   `json:"bedrock_language,omitempty"`
	DatabaseHistory []string `json:"database_history,omitempty"`
	// ClusterSecrets remembers which secret ARN the user picked for each
	// cluster ARN within this profile. The lookup is consulted before any
	// AWS-side suggestion logic so manually managed secrets (no
	// MasterUserSecret, no aws:rds:primaryDBClusterArn tag) still get a
	// one-step switch after the first manual selection.
	ClusterSecrets map[string]string `json:"cluster_secrets,omitempty"`
	// IsProduction marks this profile as a production environment so the
	// TUI can paint a distinctive warning theme (red borders, PRODUCTION
	// banner) whenever it is active. Tri-state: nil means "the user has
	// not answered yet" — the TUI prompts on first activation; a non-nil
	// value indicates the user made a deliberate choice and is not asked
	// again until they explicitly reopen the toggle.
	IsProduction *bool `json:"is_production,omitempty"`
	// IsReadOnly, when true, forces the execute path to reject any SQL
	// whose leading keyword is not a safe read operation (SELECT / WITH /
	// SHOW / EXPLAIN / DESCRIBE / DESC / TABLE / VALUES). nil defaults
	// to "enabled" at runtime so a fresh install is safe out of the box;
	// users who actively want to run writes must flip it off with F8 in
	// the TUI.
	IsReadOnly *bool `json:"is_read_only,omitempty"`
}

// Load reads the state file. A missing file yields an empty State; a malformed
// file yields an error so the caller can decide whether to fall back.
func Load() (*State, error) {
	path, err := resolvePath()
	if err != nil {
		return nil, err
	}

	s := &State{Profiles: map[string]ProfileState{}, path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read state file %s: %w", path, err)
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parse state file %s: %w", path, err)
	}
	if s.Profiles == nil {
		s.Profiles = map[string]ProfileState{}
	}
	return s, nil
}

// Get returns the state for a profile, or a zero ProfileState if absent.
// An empty profile name maps to a synthetic "_default_" key.
func (s *State) Get(profile string) ProfileState {
	return s.Profiles[profileKey(profile)]
}

// Set replaces the entry for a profile. The DatabaseHistory is normalized so
// that the most recently used database moves to the front, duplicates are
// removed, and the list is capped to historyLimit entries.
func (s *State) Set(profile string, ps ProfileState) {
	if s.Profiles == nil {
		s.Profiles = map[string]ProfileState{}
	}
	ps.DatabaseHistory = normalizeHistory(ps.DatabaseHistory, ps.Database)
	s.Profiles[profileKey(profile)] = ps
}

// Save writes the state to disk atomically (tempfile + rename) under the
// state lock. Prefer UpdateProfile for read-modify-write: Load followed by
// Save can still overwrite a change another rdq process made in between.
func (s *State) Save() error {
	path, err := s.resolvedPath()
	if err != nil {
		return err
	}
	return fsutil.WithLock(path, func() error { return s.writeTo(path) })
}

// UpdateProfile atomically applies fn to one profile's entry: it takes the
// inter-process state lock, re-reads state.json, lets fn mutate the
// profile, and writes the result back before releasing the lock. Because
// the read happens under the lock, concurrent rdq processes (TUI sessions,
// CLI) never lose each other's updates — including updates to
// other profiles, which a plain Load/Save would overwrite with stale data.
func UpdateProfile(profile string, fn func(ps *ProfileState)) error {
	path, err := resolvePath()
	if err != nil {
		return err
	}
	return fsutil.WithLock(path, func() error {
		s, err := Load()
		if err != nil {
			return err
		}
		ps := s.Get(profile)
		fn(&ps)
		s.Set(profile, ps)
		return s.writeTo(path)
	})
}

// IsReadOnly resolves the per-profile read-only policy. A profile that has
// never been toggled (nil flag) is read-only, so fresh installs are safe
// until the user explicitly opts into writes. If load fails the result is
// also read-only (fail closed) and the error is returned for logging.
func IsReadOnly(load func() (*State, error), profile string) (bool, error) {
	st, err := load()
	if err != nil {
		return true, err
	}
	ps := st.Get(profile)
	if ps.IsReadOnly == nil {
		return true, nil
	}
	return *ps.IsReadOnly, nil
}

func (s *State) resolvedPath() (string, error) {
	if s.path != "" {
		return s.path, nil
	}
	return resolvePath()
}

func (s *State) writeTo(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return fsutil.WriteFileAtomic(path, data, 0o600)
}

// resolvePath returns the state file path, honoring RDQ_STATE_FILE for tests.
func resolvePath() (string, error) {
	if override := os.Getenv("RDQ_STATE_FILE"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".rdq", "state.json"), nil
}

func profileKey(profile string) string {
	if profile == "" {
		return "_default_"
	}
	return profile
}

// normalizeHistory promotes mostRecent to the front, removes duplicates while
// preserving order, and caps the slice to historyLimit. mostRecent may be empty,
// in which case only dedup + cap are applied.
func normalizeHistory(history []string, mostRecent string) []string {
	out := make([]string, 0, len(history)+1)
	seen := map[string]struct{}{}
	if mostRecent != "" {
		out = append(out, mostRecent)
		seen[mostRecent] = struct{}{}
	}
	for _, h := range history {
		if h == "" {
			continue
		}
		if _, dup := seen[h]; dup {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
		if len(out) >= historyLimit {
			break
		}
	}
	return out
}
