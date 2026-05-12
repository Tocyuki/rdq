package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tocyuki/rdq/internal/aictx"
)

// fakeAictxStore is an in-memory stand-in for aictx.{Load,Save,Delete} so
// the handler tests do not touch ~/.rdq/aictx/ on disk.
type fakeAictxStore struct {
	entries map[string]*aictx.Context
}

func newFakeAictxStore() *fakeAictxStore {
	return &fakeAictxStore{entries: map[string]*aictx.Context{}}
}

func (f *fakeAictxStore) key(cluster, database string) string {
	return cluster + "\x00" + database
}

func (f *fakeAictxStore) load(cluster, database string) (*aictx.Context, error) {
	if c, ok := f.entries[f.key(cluster, database)]; ok {
		dup := *c
		return &dup, nil
	}
	return nil, nil
}

func (f *fakeAictxStore) save(c *aictx.Context) error {
	if c.Cluster == "" || c.Database == "" {
		return errors.New("cluster/database required")
	}
	if strings.TrimSpace(c.Content) == "" {
		return errors.New("empty content")
	}
	c.UpdatedAt = time.Now().UTC()
	dup := *c
	f.entries[f.key(c.Cluster, c.Database)] = &dup
	return nil
}

func (f *fakeAictxStore) del(cluster, database string) error {
	delete(f.entries, f.key(cluster, database))
	return nil
}

func newTestAictxHandlers() (*aictxHandlers, *fakeAictxStore) {
	store := newFakeAictxStore()
	h := newAictxHandlers()
	h.load = store.load
	h.save = store.save
	h.delete = store.del
	return h, store
}

func TestAictxGetReturnsEmptyForUnconfiguredPair(t *testing.T) {
	h, _ := newTestAictxHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/aictx?cluster=arn:c&database=app", nil)
	rr := httptest.NewRecorder()
	h.get(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var dto AiContextDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.Content != "" {
		t.Errorf("expected empty content, got %q", dto.Content)
	}
	if dto.Cluster != "arn:c" || dto.Database != "app" {
		t.Errorf("expected cluster/database echoed back, got %+v", dto)
	}
	if dto.MaxContentBytes != aictx.MaxContentBytes {
		t.Errorf("expected MaxContentBytes=%d, got %d", aictx.MaxContentBytes, dto.MaxContentBytes)
	}
}

func TestAictxGetReturnsSavedContent(t *testing.T) {
	h, store := newTestAictxHandlers()
	store.entries[store.key("arn:c", "app")] = &aictx.Context{
		Cluster: "arn:c", Database: "app",
		Content:   "active user = 30 days",
		UpdatedAt: time.Unix(1_700_000_000, 0).UTC(),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/aictx?cluster=arn:c&database=app", nil)
	rr := httptest.NewRecorder()
	h.get(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var dto AiContextDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.Content != "active user = 30 days" {
		t.Errorf("content mismatch: %q", dto.Content)
	}
	if dto.UpdatedAt == "" {
		t.Errorf("expected updatedAt, got empty")
	}
}

func TestAictxGetRejectsMissingQueryParams(t *testing.T) {
	h, _ := newTestAictxHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/aictx?cluster=arn:c", nil)
	rr := httptest.NewRecorder()
	h.get(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAictxPutSavesContent(t *testing.T) {
	h, store := newTestAictxHandlers()
	body := `{"cluster":"arn:c","database":"app","content":"hello"}`
	req := httptest.NewRequest(http.MethodPut, "/api/aictx", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.put(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	saved := store.entries[store.key("arn:c", "app")]
	if saved == nil || saved.Content != "hello" {
		t.Errorf("expected saved content 'hello', got %+v", saved)
	}
}

func TestAictxPutRejectsEmptyContent(t *testing.T) {
	h, _ := newTestAictxHandlers()
	body := `{"cluster":"arn:c","database":"app","content":""}`
	req := httptest.NewRequest(http.MethodPut, "/api/aictx", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.put(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAictxPutRejectsMissingKeys(t *testing.T) {
	h, _ := newTestAictxHandlers()
	body := `{"cluster":"","database":"app","content":"x"}`
	req := httptest.NewRequest(http.MethodPut, "/api/aictx", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.put(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestAictxDeleteRemovesEntry(t *testing.T) {
	h, store := newTestAictxHandlers()
	store.entries[store.key("arn:c", "app")] = &aictx.Context{Cluster: "arn:c", Database: "app", Content: "x"}
	req := httptest.NewRequest(http.MethodDelete, "/api/aictx?cluster=arn:c&database=app", nil)
	rr := httptest.NewRecorder()
	h.del(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if _, ok := store.entries[store.key("arn:c", "app")]; ok {
		t.Errorf("expected entry to be removed")
	}
}
