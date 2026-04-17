package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, HealthDTO{Status: "ok"})
	})
}

func TestCheckOriginAllowsKnownOrigin(t *testing.T) {
	mw := checkOrigin([]string{"http://127.0.0.1:8080"})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	req.Host = "127.0.0.1:8080"
	rr := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCheckOriginRejectsUnknownOrigin(t *testing.T) {
	mw := checkOrigin([]string{"http://127.0.0.1:8080"})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Origin", "http://evil.example")
	req.Host = "127.0.0.1:8080"
	rr := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
	var body ErrorDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid error body: %v", err)
	}
	if body.Error.Code != errCodeOriginDenied {
		t.Errorf("expected code %s, got %s", errCodeOriginDenied, body.Error.Code)
	}
}

func TestCheckOriginAllowsMissingOriginWithLocalhostHost(t *testing.T) {
	mw := checkOrigin([]string{"http://127.0.0.1:8080"})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Host = "localhost:8080"
	rr := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCheckOriginRejectsMissingOriginWithRemoteHost(t *testing.T) {
	mw := checkOrigin([]string{"http://127.0.0.1:8080"})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Host = "rdq.example.com:8080"
	rr := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCheckOriginSkipsNonAPIRoutes(t *testing.T) {
	mw := checkOrigin([]string{"http://127.0.0.1:8080"})
	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	req.Header.Set("Origin", "http://evil.example")
	rr := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected non-API request to pass through, got %d", rr.Code)
	}
}

func TestRecoverPanicReturns500(t *testing.T) {
	panicHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	recoverPanic(panicHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if !strings.Contains(string(body), errCodeInternal) {
		t.Errorf("expected body to mention %s, got %s", errCodeInternal, body)
	}
}

func TestIsLocalhostHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.1:8080", true},
		{"localhost", true},
		{"localhost:5173", true},
		{"::1", true},
		{"", true},
		{"example.com", false},
		{"10.0.0.5:8080", false},
	}
	for _, tc := range cases {
		if got := isLocalhostHost(tc.host); got != tc.want {
			t.Errorf("isLocalhostHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}
