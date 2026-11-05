package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/Tocyuki/rdq/internal/history"
	"github.com/Tocyuki/rdq/internal/runner"
	"github.com/Tocyuki/rdq/internal/state"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// executeHandlers owns POST /api/execute. It holds collaborators (awsCache,
// history store, runner function) so tests can swap each independently.
type executeHandlers struct {
	awsCache *awsCache
	history  *history.Store

	// executeSQL is a seam for tests so the handler can be exercised
	// without an actual Data API round trip. The default is
	// runner.ExecuteSQLWithConfig.
	executeSQL func(ctx context.Context, cfg aws.Config, target runner.Target, sql string) (*runner.Result, time.Duration, error)

	// loadState is a seam for tests + the source of truth for the
	// per-profile read-only policy. The execute handler re-reads
	// state.json on every request (authoritative over any client-supplied
	// value) so toggling the flag in Settings takes effect immediately
	// without depending on the SPA to resend it.
	loadState func() (*state.State, error)
}

func newExecuteHandlers(cache *awsCache, hist *history.Store) *executeHandlers {
	return &executeHandlers{
		awsCache:   cache,
		history:    hist,
		executeSQL: runner.ExecuteSQLWithConfig,
		loadState:  state.Load,
	}
}

// execute runs a single SQL statement against the requested connection.
// History is recorded whether the call succeeds or fails, matching the TUI's
// behaviour so the SPA's history panel sees both branches. Ephemeral runs
// (profile == "") skip history recording.
func (h *executeHandlers) execute(w http.ResponseWriter, r *http.Request) {
	var req ExecuteRequest
	if !decodeJSONBody(w, r, &req, maxExecuteBodyBytes) {
		return
	}
	if req.Profile == "" {
		// The SPA always has a profile; missing means a programmer error
		// worth surfacing loudly. (Ephemeral mode is a CLI concern.)
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"profile is required")
		return
	}
	if req.Cluster == "" || req.Secret == "" || req.Database == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"cluster, secret, and database are required")
		return
	}

	// Read-only gate: checked before touching AWS so a destructive
	// attempt never even opens an SDK handle.
	if h.isReadOnlyProfile(req.Profile) && !runner.IsReadOnlySQL(req.SQL) {
		h.recordHistory(req, runner.ErrWriteBlocked, 0)
		writeJSONError(w, http.StatusForbidden, errCodeReadOnly,
			"read-only mode is on for this profile; toggle it off in Settings to run writes")
		return
	}

	// Destructive-statement gate — see ExecuteResponse.NeedsConfirmation
	// for the 200-not-4xx contract, and runner.NeedsConfirmation for the
	// classification rules. Unconfirmed attempts are deliberately NOT
	// recorded in history so the picker does not fill with cancelled
	// prompts.
	if !req.Confirmed {
		if need, reason := runner.NeedsConfirmation(req.SQL); need {
			writeJSON(w, ExecuteResponse{
				Columns:           []string{},
				Rows:              [][]any{},
				NeedsConfirmation: true,
				ConfirmReason:     reason,
			})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), runner.ExecuteTimeout)
	defer cancel()

	cfg, err := h.awsCache.Get(ctx, req.Profile)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}

	target := runner.Target{
		Profile:  req.Profile,
		Region:   cfg.Region,
		Cluster:  req.Cluster,
		Secret:   req.Secret,
		Database: req.Database,
	}
	res, elapsed, execErr := h.executeSQL(ctx, cfg, target, req.SQL)

	// Record history unconditionally (success or failure) so the SPA
	// picker matches the TUI's behaviour.
	h.recordHistory(req, execErr, elapsed)

	if execErr != nil {
		status, code := http.StatusBadGateway, errCodeAWSError
		if errors.Is(execErr, runner.ErrEmptySQL) {
			status, code = http.StatusBadRequest, errCodeBadRequest
		} else if errors.Is(execErr, context.DeadlineExceeded) {
			status, code = http.StatusGatewayTimeout, errCodeTimeout
		}
		writeJSONError(w, status, code, execErr.Error())
		return
	}

	resp := ExecuteResponse{
		Columns:    res.Columns,
		Rows:       res.Rows,
		Updated:    res.Updated,
		DurationMS: elapsed.Milliseconds(),
	}
	if resp.Columns == nil {
		resp.Columns = []string{}
	}
	if resp.Rows == nil {
		resp.Rows = [][]any{}
	}
	writeJSON(w, resp)
}

// isReadOnlyProfile resolves the per-profile read-only policy from
// state.json via state.IsReadOnly (nil flag or load failure → read-only).
func (h *executeHandlers) isReadOnlyProfile(profile string) bool {
	if h.loadState == nil {
		return true
	}
	readOnly, err := state.IsReadOnly(h.loadState, profile)
	if err != nil {
		// Failing closed is safer than failing open for a
		// destructive-statement guard.
		log.Printf("rdq gui: state.Load failed during read-only check: %v", err)
	}
	return readOnly
}

// recordHistory writes a one-line JSONL entry to the history store. All
// failures are logged because a broken history file should not prevent the
// user from seeing their query result.
func (h *executeHandlers) recordHistory(req ExecuteRequest, execErr error, elapsed time.Duration) {
	if err := h.history.Record(req.Profile, req.Database, req.SQL, execErr, elapsed); err != nil {
		log.Printf("rdq gui: append history: %v", err)
	}
}
