package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Tocyuki/rdq/internal/history"
)

// historyHandlers serve the SPA's history panel: listing entries and
// toggling the star flag. Writes go through the same history.Store the
// execute handler appends to, so the two are consistent.
type historyHandlers struct {
	store *history.Store
}

func newHistoryHandlers(store *history.Store) *historyHandlers {
	return &historyHandlers{store: store}
}

// list returns history entries for the requested profile and database,
// ordered most recent first. When profile or database is empty we return an
// empty list rather than 400 because a fresh install has nothing to show.
func (h *historyHandlers) list(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, HistoryDTO{Entries: []HistoryEntryDTO{}})
		return
	}
	profile := r.URL.Query().Get("profile")
	database := r.URL.Query().Get("database")
	if profile == "" || database == "" {
		writeJSON(w, HistoryDTO{Entries: []HistoryEntryDTO{}})
		return
	}
	entries, err := h.store.Load(profile, database)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, errCodeInternal,
			"load history: "+err.Error())
		return
	}
	dto := HistoryDTO{Entries: make([]HistoryEntryDTO, 0, len(entries))}
	for _, e := range entries {
		dto.Entries = append(dto.Entries, HistoryEntryDTO{
			Profile:    e.Profile,
			Database:   e.Database,
			SQL:        e.SQL,
			At:         e.At.UTC().Format(time.RFC3339Nano),
			Ok:         e.Ok,
			DurationMS: e.DurationMS,
			Error:      e.ErrorMsg,
			Favorite:   e.Favorite,
		})
	}
	writeJSON(w, dto)
}

// favorite toggles (or explicitly sets) the star flag on the entry
// identified by its RFC3339Nano timestamp.
func (h *historyHandlers) favorite(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSONError(w, http.StatusInternalServerError, errCodeInternal,
			"history store not available")
		return
	}
	var req FavoriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"invalid JSON body: "+err.Error())
		return
	}
	at, err := time.Parse(time.RFC3339Nano, req.At)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"at must be RFC3339Nano: "+err.Error())
		return
	}
	if err := h.store.SetFavorite(at, req.Favorite); err != nil {
		writeJSONError(w, http.StatusInternalServerError, errCodeInternal,
			"update favorite: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
