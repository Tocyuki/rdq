package server

import (
	"encoding/json"
	"net/http"

	"github.com/Tocyuki/rdq/internal/awsauth"
)

// sessionHandlers serves the minimal set of endpoints needed before the SPA
// can make any other call: health ping, the current connection pointer, and
// the list of AWS profiles configured on the machine.
type sessionHandlers struct {
	session *sessionStore

	// listProfiles is a seam for tests so we do not read real ~/.aws files.
	listProfiles func() ([]string, error)
}

func newSessionHandlers(session *sessionStore) *sessionHandlers {
	return &sessionHandlers{
		session:      session,
		listProfiles: awsauth.ListProfiles,
	}
}

// health responds with a tiny JSON body that Kubernetes-style probes and
// smoke tests can rely on.
func (h *sessionHandlers) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, HealthDTO{Status: "ok"})
}

// getSession returns the currently selected profile / cluster / secret / db
// as seeded from CLI flags + state.json on startup, or whatever the SPA most
// recently saved via PUT.
func (h *sessionHandlers) getSession(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.session.Get())
}

// putSession replaces the session wholesale and writes it back to state.json
// (unless the profile is empty, which means ephemeral mode and skips disk).
// All fields are taken verbatim from the request body; no merging with the
// previous value, so the SPA sends a complete SessionDTO.
func (h *sessionHandlers) putSession(w http.ResponseWriter, r *http.Request) {
	var dto SessionDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"invalid JSON body: "+err.Error())
		return
	}
	h.session.Set(dto)
	if err := h.session.PersistToState(); err != nil {
		// Persistence failure is not fatal — the in-memory session is
		// still updated and the SPA can continue. We surface it as a
		// warning in the log so the operator can investigate.
		writeJSONError(w, http.StatusInternalServerError, errCodeInternal,
			"could not persist session to state.json: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// profiles returns the AWS profile names configured in the local config /
// credentials files. These are purely labels — the server does not attempt
// to load or validate them until another endpoint asks for a Config.
func (h *sessionHandlers) profiles(w http.ResponseWriter, _ *http.Request) {
	profiles, err := h.listProfiles()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, errCodeInternal,
			"could not list AWS profiles: "+err.Error())
		return
	}
	if profiles == nil {
		profiles = []string{}
	}
	writeJSON(w, ProfilesDTO{Profiles: profiles})
}
