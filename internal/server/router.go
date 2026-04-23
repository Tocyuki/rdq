package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/Tocyuki/rdq/internal/history"
)

// buildRouter wires the API routes under /api/, then serves the SPA from the
// embedded dist filesystem. Any path that is neither an /api/* call nor an
// existing static asset falls back to dist/index.html so React Router routes
// like /history or /schema resolve when typed into the address bar.
func buildRouter(deps handlerDeps) http.Handler {
	mux := http.NewServeMux()

	// API routes. Using the Go 1.22+ method-qualified pattern syntax so we
	// can bind verbs without a third-party router.
	sess := newSessionHandlers(deps.session)
	mux.HandleFunc("GET /api/health", sess.health)
	mux.HandleFunc("GET /api/session", sess.getSession)
	mux.HandleFunc("PUT /api/session", sess.putSession)
	mux.HandleFunc("GET /api/profiles", sess.profiles)

	conn := newConnectionHandlers(deps.awsCache)
	mux.HandleFunc("GET /api/clusters", conn.clusters)
	mux.HandleFunc("GET /api/secrets", conn.secrets)
	mux.HandleFunc("GET /api/databases", conn.databases)

	exec := newExecuteHandlers(deps.awsCache, deps.history)
	mux.HandleFunc("POST /api/execute", exec.execute)

	sch := newSchemaHandlers(deps.awsCache)
	mux.HandleFunc("GET /api/schema", sch.get)
	mux.HandleFunc("POST /api/schema/refresh", sch.refresh)

	hist := newHistoryHandlers(deps.history)
	mux.HandleFunc("GET /api/history", hist.list)
	mux.HandleFunc("POST /api/history/favorite", hist.favorite)

	ai := newAIHandlers(deps.awsCache)
	mux.HandleFunc("GET /api/ai/models", ai.models)
	mux.HandleFunc("POST /api/ai/ask", ai.ask)
	mux.HandleFunc("POST /api/ai/explain", ai.explain)
	mux.HandleFunc("POST /api/ai/review", ai.review)
	mux.HandleFunc("POST /api/ai/analyze", ai.analyze)

	// SPA static + fallback handler. Mounted on the root so it catches
	// everything the API router did not.
	mux.Handle("/", spaHandler(deps.distFS))

	// Middleware order: log (outermost) → recover → origin check → API token
	// check → mux.
	return chain(mux, logRequest, recoverPanic, checkOrigin(deps.allowedOrigins), requireAPIToken(deps.apiToken))
}

// handlerDeps bundles dependencies the router needs to wire up handlers. It
// keeps buildRouter's signature small and lets tests construct a lightweight
// variant without Options plumbing.
type handlerDeps struct {
	session        *sessionStore
	awsCache       *awsCache
	history        *history.Store
	distFS         fs.FS
	allowedOrigins []string
	apiToken       string
}

// spaHandler serves files from distFS, falling back to index.html for any
// path that does not map to a real file so React Router paths survive a page
// reload. It never serves the SPA bundle for /api/* — those paths should
// have already been matched by the API router; reaching spaHandler with an
// /api/* path is a route-miss and returns a 404 JSON.
func spaHandler(distFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONError(w, http.StatusNotFound, errCodeNotFound,
				"no such API endpoint: "+r.URL.Path)
			return
		}
		// Strip the leading slash for fs.Stat.
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean == "" {
			clean = "index.html"
		}
		if _, err := fs.Stat(distFS, clean); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Fallback to index.html so SPA routes like /query, /history,
		// /schema render correctly on reload and direct navigation.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
