package server

import (
	"sync"

	"github.com/Tocyuki/rdq/internal/state"
)

// sessionStore holds the server's view of the user's "current connection"
// selection. It is seeded from Options (typically from rdq CLI flags +
// state.json on startup) and updated by PUT /api/session.
//
// The handlers themselves are stateless — every mutating endpoint receives
// the profile/cluster/secret/database in its request body. The session is
// only consulted for the reload-survival path (`GET /api/session`) and for
// persistence to state.json on explicit save.
type sessionStore struct {
	mu    sync.RWMutex
	inner SessionDTO
}

func newSessionStore(seed SessionDTO) *sessionStore {
	return &sessionStore{inner: seed}
}

// Get returns a snapshot of the current session. The returned value is a
// copy, so callers can read fields without holding the lock.
func (s *sessionStore) Get() SessionDTO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inner
}

// Set replaces the session state wholesale.
func (s *sessionStore) Set(v SessionDTO) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inner = v
}

// PersistToState writes the session back to the per-profile state.json entry.
// Ephemeral runs (profile == "") skip disk writes entirely so no traces are
// left when the user is using direct AWS_ACCESS_KEY_ID credentials.
func (s *sessionStore) PersistToState() error {
	snap := s.Get()
	if snap.Profile == "" {
		return nil
	}
	st, err := state.Load()
	if err != nil {
		return err
	}
	ps := st.Get(snap.Profile)
	ps.Cluster = snap.Cluster
	ps.Secret = snap.Secret
	ps.Database = snap.Database
	if snap.BedrockModel != "" {
		ps.BedrockModel = snap.BedrockModel
	}
	if snap.BedrockLanguage != "" {
		ps.BedrockLanguage = snap.BedrockLanguage
	}
	if ps.ClusterSecrets == nil {
		ps.ClusterSecrets = map[string]string{}
	}
	if snap.Cluster != "" && snap.Secret != "" {
		ps.ClusterSecrets[snap.Cluster] = snap.Secret
	}
	st.Set(snap.Profile, ps)
	return st.Save()
}

// LoadFromState enriches a session seed with values persisted under the
// profile's state.json entry. Explicit seed values take priority; the state
// fills only the blanks. This is how startup works: CLI flags (seed) win
// over cached state (seed fallback).
func LoadFromState(seed SessionDTO) SessionDTO {
	if seed.Profile == "" {
		return seed
	}
	st, err := state.Load()
	if err != nil {
		return seed
	}
	ps := st.Get(seed.Profile)
	if seed.Cluster == "" {
		seed.Cluster = ps.Cluster
	}
	if seed.Secret == "" {
		seed.Secret = ps.Secret
	}
	if seed.Database == "" {
		seed.Database = ps.Database
	}
	if seed.BedrockModel == "" {
		seed.BedrockModel = ps.BedrockModel
	}
	if seed.BedrockLanguage == "" {
		seed.BedrockLanguage = ps.BedrockLanguage
	}
	return seed
}
