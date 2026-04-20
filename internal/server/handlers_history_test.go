package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tocyuki/rdq/internal/history"
)

func seedHistory(t *testing.T) (*history.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	t.Setenv("RDQ_HISTORY_FILE", path)
	store, err := history.New()
	if err != nil {
		t.Fatal(err)
	}
	return store, path
}

func TestHistoryListEmptyWhenProfileOrDatabaseMissing(t *testing.T) {
	store, _ := seedHistory(t)
	h := newHistoryHandlers(store)
	req := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	rr := httptest.NewRecorder()
	h.list(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body HistoryDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Entries) != 0 {
		t.Errorf("expected empty list, got %v", body.Entries)
	}
}

func TestHistoryListReturnsEntries(t *testing.T) {
	store, _ := seedHistory(t)
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	if err := store.Append(history.Entry{
		Profile: "dev", Database: "app", SQL: "SELECT 1",
		At: now, Ok: true, DurationMS: 10,
	}); err != nil {
		t.Fatal(err)
	}
	h := newHistoryHandlers(store)
	req := httptest.NewRequest(http.MethodGet, "/api/history?profile=dev&database=app", nil)
	rr := httptest.NewRecorder()
	h.list(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body HistoryDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Entries) != 1 || body.Entries[0].SQL != "SELECT 1" {
		t.Errorf("unexpected entries: %+v", body.Entries)
	}
}

func TestHistoryFavoriteToggle(t *testing.T) {
	store, _ := seedHistory(t)
	at := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	if err := store.Append(history.Entry{
		Profile: "dev", Database: "app", SQL: "SELECT 1", At: at, Ok: true,
	}); err != nil {
		t.Fatal(err)
	}
	h := newHistoryHandlers(store)

	payload := FavoriteRequest{At: at.Format(time.RFC3339Nano), Favorite: true}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/history/favorite", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.favorite(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	entries, err := store.Load("dev", "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].Favorite {
		t.Errorf("favorite not persisted: %+v", entries)
	}
}

func TestHistoryFavoriteRejectsInvalidTimestamp(t *testing.T) {
	store, _ := seedHistory(t)
	h := newHistoryHandlers(store)
	payload := FavoriteRequest{At: "yesterday", Favorite: true}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/history/favorite", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.favorite(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}
