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

// Save writes the state to disk atomically (tempfile + rename).
func (s *State) Save() error {
	path := s.path
	if path == "" {
		var err error
		path, err = resolvePath()
		if err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*.json")
	if err != nil {
		return fmt.Errorf("create temp state file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp state file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp state file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename state file: %w", err)
	}
	return nil
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
