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
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var got SessionDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if got.Profile != payload.Profile || got.Cluster != payload.Cluster || got.Secret != payload.Secret || got.Database != payload.Database {
		t.Errorf("response session = %+v, want profile/cluster/secret/database to match %+v", got, payload)
	}
	if mem := h.session.Get(); mem.Profile != payload.Profile {
		t.Errorf("in-memory session profile = %q, want %q", mem.Profile, payload.Profile)
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

func TestPutSessionProductionFlagRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_STATE_FILE", filepath.Join(dir, "state.json"))

	h := newTestSessionHandlers(SessionDTO{}, nil)
	trueVal := true
	payload := SessionDTO{
		Profile:      "dev",
		Cluster:      "arn:c",
		Secret:       "arn:s",
		Database:     "app",
		IsProduction: &trueVal,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/session", strings.NewReader(string(body)))
	rr := httptest.NewRecorder()
	h.putSession(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	// Reload seed: LoadFromState should recover IsProduction=true.
	seeded := LoadFromState(SessionDTO{Profile: "dev"})
	if seeded.IsProduction == nil || !*seeded.IsProduction {
		t.Errorf("expected IsProduction=true after state.json round trip, got %v", seeded.IsProduction)
	}
}

func TestPutSessionAutoRunReadOnlyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_STATE_FILE", filepath.Join(dir, "state.json"))

	h := newTestSessionHandlers(SessionDTO{}, nil)
	trueVal := true
	payload := SessionDTO{
		Profile:         "dev",
		Cluster:         "arn:c",
		Secret:          "arn:s",
		Database:        "app",
		AutoRunReadOnly: &trueVal,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/session", strings.NewReader(string(body)))
	rr := httptest.NewRecorder()
	h.putSession(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	// LoadFromState should recover AutoRunReadOnly=true after persistence
	// and apply the same tri-state merge as IsProduction / IsReadOnly:
	// a nil delta on the seed lets the stored value bubble back up.
	seeded := LoadFromState(SessionDTO{Profile: "dev"})
	if seeded.AutoRunReadOnly == nil || !*seeded.AutoRunReadOnly {
		t.Errorf("expected AutoRunReadOnly=true after state.json round trip, got %v", seeded.AutoRunReadOnly)
	}
}

func TestPutSessionAutoRunReadOnlyHonorsExplicitFalse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_STATE_FILE", filepath.Join(dir, "state.json"))

	h := newTestSessionHandlers(SessionDTO{}, nil)

	// Save true first, then override with explicit false. The flag must
	// land on disk as false (not nil) so the SPA toggle round-trips
	// correctly.
	trueVal := true
	first, _ := json.Marshal(SessionDTO{
		Profile: "dev", Cluster: "arn:c", Secret: "arn:s", Database: "app",
		AutoRunReadOnly: &trueVal,
	})
	rr := httptest.NewRecorder()
	h.putSession(rr, httptest.NewRequest(http.MethodPut, "/api/session", strings.NewReader(string(first))))
	if rr.Code != http.StatusOK {
		t.Fatalf("first put status = %d", rr.Code)
	}

	falseVal := false
	second, _ := json.Marshal(SessionDTO{
		Profile: "dev", Cluster: "arn:c", Secret: "arn:s", Database: "app",
		AutoRunReadOnly: &falseVal,
	})
	rr = httptest.NewRecorder()
	h.putSession(rr, httptest.NewRequest(http.MethodPut, "/api/session", strings.NewReader(string(second))))
	if rr.Code != http.StatusOK {
		t.Fatalf("second put status = %d", rr.Code)
	}

	seeded := LoadFromState(SessionDTO{Profile: "dev"})
	if seeded.AutoRunReadOnly == nil {
		t.Fatal("expected AutoRunReadOnly to be persisted as explicit false, got nil")
	}
	if *seeded.AutoRunReadOnly {
		t.Errorf("expected AutoRunReadOnly=false after explicit override, got true")
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
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
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
