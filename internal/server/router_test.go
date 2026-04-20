package server

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// routerTestFS mocks the SPA dist so tests do not depend on the real Vite
// build output shipped via //go:embed.
func routerTestFS() fs.FS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><html><body>rdq</body></html>")},
		"assets/app.js": {Data: []byte("console.log('rdq')")},
	}
}

func newTestRouter() http.Handler {
	return buildRouter(handlerDeps{
		session:        newSessionStore(SessionDTO{Profile: "seed"}),
		awsCache:       newAWSCache(),
		distFS:         routerTestFS(),
		allowedOrigins: []string{"http://127.0.0.1:8080"},
	})
}

func TestRouterAPIHealth(t *testing.T) {
	ts := httptest.NewServer(newTestRouter())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/health", nil)
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	// NewRequest sets Host to the server URL which is not localhost:8080;
	// the allow list contains 127.0.0.1:8080 explicitly via Origin, so the
	// middleware accepts on the Origin match.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
}

func TestRouterSessionRoundTrip(t *testing.T) {
	ts := httptest.NewServer(newTestRouter())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/session", nil)
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/session status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "\"profile\":\"seed\"") {
		t.Errorf("expected seed profile, got %s", body)
	}
}

func TestRouterServesSPA(t *testing.T) {
	ts := httptest.NewServer(newTestRouter())
	defer ts.Close()

	// Existing static asset → served directly.
	resp, err := http.Get(ts.URL + "/assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("assets/app.js status = %d", resp.StatusCode)
	}

	// SPA-style route → falls back to index.html.
	resp2, err := http.Get(ts.URL + "/query")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("/query status = %d", resp2.StatusCode)
	}
	body, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body), "<!doctype html>") {
		t.Errorf("expected index.html fallback, got %s", body)
	}
}

func TestRouterUnknownAPIReturns404JSON(t *testing.T) {
	ts := httptest.NewServer(newTestRouter())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/does-not-exist", nil)
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), errCodeNotFound) {
		t.Errorf("expected JSON error body, got %s", body)
	}
}
