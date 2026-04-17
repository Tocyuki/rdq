package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestSessionHandlers(seed SessionDTO, profiles []string) *sessionHandlers {
	h := newSessionHandlers(newSessionStore(seed))
	h.listProfiles = func() ([]string, error) { return profiles, nil }
	return h
}

func TestHealthEndpoint(t *testing.T) {
	h := newTestSessionHandlers(SessionDTO{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	h.health(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body HealthDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}

func TestGetSessionReturnsSeed(t *testing.T) {
	seed := SessionDTO{Profile: "dev", Cluster: "arn:c", Secret: "arn:s", Database: "app"}
	h := newTestSessionHandlers(seed, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	rr := httptest.NewRecorder()
	h.getSession(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body SessionDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body != seed {
		t.Errorf("session = %+v, want %+v", body, seed)
	}
}

func TestPutSessionUpdatesAndPersists(t *testing.T) {
	// Redirect state.json to a temp file so the test doesn't touch the
	// developer's real ~/.rdq/state.json.
	dir := t.TempDir()
	t.Setenv("RDQ_STATE_FILE", filepath.Join(dir, "state.json"))

	h := newTestSessionHandlers(SessionDTO{}, nil)

	payload := SessionDTO{
		Profile:         "dev",
		Cluster:         "arn:aws:rds:us-east-1:1:cluster:demo",
		Secret:          "arn:aws:secretsmanager:us-east-1:1:secret:demo",
		Database:        "app",
		BedrockModel:    "anthropic.claude-sonnet-4-5-v1:0",
		BedrockLanguage: "Japanese",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/session", strings.NewReader(string(body)))
	rr := httptest.NewRecorder()
	h.putSession(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rr.Code, rr.Body.String())
	}
	if got := h.session.Get(); got != payload {
		t.Errorf("session = %+v, want %+v", got, payload)
	}
	// state.json should now exist and contain the profile entry.
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("state.json not written: %v", err)
	}
	if !strings.Contains(string(data), "\"dev\"") {
		t.Errorf("expected state.json to include the dev profile entry:\n%s", data)
	}
}

func TestPutSessionRejectsInvalidJSON(t *testing.T) {
	h := newTestSessionHandlers(SessionDTO{}, nil)
	req := httptest.NewRequest(http.MethodPut, "/api/session", strings.NewReader("{not-json"))
	rr := httptest.NewRecorder()
	h.putSession(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	var body ErrorDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid error body: %v", err)
	}
	if body.Error.Code != errCodeBadRequest {
		t.Errorf("code = %s, want %s", body.Error.Code, errCodeBadRequest)
	}
}

func TestPutSessionEphemeralDoesNotWriteState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	t.Setenv("RDQ_STATE_FILE", statePath)

	h := newTestSessionHandlers(SessionDTO{}, nil)

	payload := SessionDTO{Cluster: "arn:c", Secret: "arn:s", Database: "app"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/session", strings.NewReader(string(body)))
	rr := httptest.NewRecorder()
	h.putSession(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("expected state.json to be untouched for empty profile, got err = %v", err)
	}
}

func TestProfilesEndpoint(t *testing.T) {
	want := []string{"dev", "prod"}
	h := newTestSessionHandlers(SessionDTO{}, want)
	req := httptest.NewRequest(http.MethodGet, "/api/profiles", nil)
	rr := httptest.NewRecorder()
	h.profiles(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body ProfilesDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(body.Profiles) != len(want) || body.Profiles[0] != "dev" || body.Profiles[1] != "prod" {
		t.Errorf("profiles = %v, want %v", body.Profiles, want)
	}
}

func TestProfilesEndpointEmpty(t *testing.T) {
	h := newTestSessionHandlers(SessionDTO{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/profiles", nil)
	rr := httptest.NewRecorder()
	h.profiles(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	// Empty list should still render as [] not null.
	if strings.Contains(rr.Body.String(), "null") {
		t.Errorf("expected [], got %s", rr.Body.String())
	}
}
