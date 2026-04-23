package server

import (
	"crypto/subtle"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

const apiTokenHeader = "X-RDQ-Token"

// checkOrigin returns a middleware that rejects /api/* requests whose Origin
// header is not in allowedOrigins. Requests without an Origin header are
// permitted only when the Host header resolves to a localhost address, which
// keeps curl usage from the developer's machine working while still blocking
// drive-by browser navigations (which always include Origin).
//
// DNS rebinding is countered by pinning the Host-only path to loopback names
// — a rebind would change Host to something else.
func checkOrigin(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			origin := r.Header.Get("Origin")
			if origin == "" {
				if !isLocalhostHost(r.Host) {
					writeJSONError(w, http.StatusForbidden, errCodeOriginDenied,
						"Host header must be localhost or 127.0.0.1")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := allowed[origin]; !ok {
				writeJSONError(w, http.StatusForbidden, errCodeOriginDenied,
					fmt.Sprintf("origin %q is not allowed", origin))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireAPIToken rejects /api/* requests that do not present the current GUI
// session token. The token is delivered only through the launch URL fragment
// that rdq opens in the browser, so another local process cannot use the
// loopback API just by knowing the port.
func requireAPIToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			got := r.Header.Get(apiTokenHeader)
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				writeJSONError(w, http.StatusUnauthorized, errCodeUnauthorized,
					"missing or invalid GUI session token; reopen rdq gui and retry")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isLocalhostHost returns true when host (with or without port) resolves to
// the loopback address family.
func isLocalhostHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	switch h {
	case "127.0.0.1", "localhost", "::1", "":
		return true
	}
	return false
}

// recoverPanic converts a handler panic into a 500 JSON response while
// logging the stack to the server log. It is the last line of defence for
// unexpected nil derefs, array out-of-bounds, etc.
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("rdq gui: panic in %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				writeJSONError(w, http.StatusInternalServerError, errCodeInternal,
					"internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder is a small wrapper around http.ResponseWriter so logRequest
// can see the status code the handler wrote (the stdlib interface does not
// expose it otherwise).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// logRequest logs method / path / status / duration for every request. It is
// deliberately terse: the goal is a one-line audit trail for developers
// tailing the server output, not structured observability.
func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("rdq gui: %s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

// chain applies middlewares left-to-right so the first listed wraps the
// outermost layer (runs first on ingress, last on egress).
func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
