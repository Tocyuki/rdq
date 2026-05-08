package server

import (
	"net/http"
	"time"

	"github.com/Tocyuki/rdq/internal/aictx"
)

// aictxHandlers serve the per-(cluster, database) prompt context that is
// injected into Bedrock system prompts to improve NL→SQL accuracy.
//
// All three verbs (GET / PUT / DELETE) use the same (cluster, database)
// pair as the lookup key; the SPA does not need to pass a profile because
// the context is stored on disk independently of any AWS profile.
type aictxHandlers struct {
	// DI seams for tests so they don't read or write ~/.rdq/aictx/.
	load   func(cluster, database string) (*aictx.Context, error)
	save   func(c *aictx.Context) error
	delete func(cluster, database string) error
}

func newAictxHandlers() *aictxHandlers {
	return &aictxHandlers{
		load:   aictx.Load,
		save:   aictx.Save,
		delete: aictx.Delete,
	}
}

// get serves GET /api/aictx?cluster=...&database=... — returns the saved
// context or an empty one when nothing is configured yet.
func (h *aictxHandlers) get(w http.ResponseWriter, r *http.Request) {
	cluster := r.URL.Query().Get("cluster")
	database := r.URL.Query().Get("database")
	if cluster == "" || database == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"cluster and database query parameters are required")
		return
	}
	c, err := h.load(cluster, database)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, errCodeInternal, err.Error())
		return
	}
	if c == nil {
		writeJSON(w, AiContextDTO{
			Cluster:         cluster,
			Database:        database,
			MaxContentBytes: aictx.MaxContentBytes,
		})
		return
	}
	writeJSON(w, AiContextDTO{
		Cluster:         c.Cluster,
		Database:        c.Database,
		Content:         c.Content,
		UpdatedAt:       c.UpdatedAt.UTC().Format(time.RFC3339Nano),
		MaxContentBytes: aictx.MaxContentBytes,
	})
}

// put serves PUT /api/aictx — saves the context for (cluster, database).
// Empty content is rejected; the SPA should call DELETE instead.
func (h *aictxHandlers) put(w http.ResponseWriter, r *http.Request) {
	var req PutAiContextRequest
	if !decodeJSONBody(w, r, &req, maxAictxBodyBytes) {
		return
	}
	if req.Cluster == "" || req.Database == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"cluster and database are required")
		return
	}
	c := &aictx.Context{
		Cluster:  req.Cluster,
		Database: req.Database,
		Content:  req.Content,
	}
	if err := h.save(c); err != nil {
		// Save returns errors for empty / oversized content too; surface
		// them as a 400 so the SPA can show a friendly message.
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest, err.Error())
		return
	}
	writeJSON(w, AiContextDTO{
		Cluster:         c.Cluster,
		Database:        c.Database,
		Content:         c.Content,
		UpdatedAt:       c.UpdatedAt.UTC().Format(time.RFC3339Nano),
		MaxContentBytes: aictx.MaxContentBytes,
	})
}

// del serves DELETE /api/aictx?cluster=...&database=... — removes the saved
// context. Missing files are treated as success.
func (h *aictxHandlers) del(w http.ResponseWriter, r *http.Request) {
	cluster := r.URL.Query().Get("cluster")
	database := r.URL.Query().Get("database")
	if cluster == "" || database == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"cluster and database query parameters are required")
		return
	}
	// aictx.Delete already swallows os.ErrNotExist, so any error reaching
	// here is a real disk failure worth surfacing.
	if err := h.delete(cluster, database); err != nil {
		writeJSONError(w, http.StatusInternalServerError, errCodeInternal, err.Error())
		return
	}
	writeJSON(w, AiContextDTO{
		Cluster:         cluster,
		Database:        database,
		MaxContentBytes: aictx.MaxContentBytes,
	})
}
