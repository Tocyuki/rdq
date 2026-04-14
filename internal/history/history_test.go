package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setHistoryPath(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	t.Setenv("RDQ_HISTORY_FILE", path)
	store, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store
}

func TestAppendLoadRoundTrip(t *testing.T) {
	store := setHistoryPath(t)
	at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

	if err := store.Append(Entry{
		Profile: "dev", Database: "myapp",
		SQL: "SELECT 1", At: at, Ok: true, DurationMS: 12,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(Entry{
		Profile: "dev", Database: "myapp",
		SQL: "SELECT * FROM users", At: at.Add(time.Minute), Ok: true, DurationMS: 84,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load("dev", "myapp")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].SQL != "SELECT * FROM users" {
		t.Errorf("expected newest-first ordering, got %q at index 0", got[0].SQL)
	}
	if got[1].SQL != "SELECT 1" {
		t.Errorf("expected oldest at index 1, got %q", got[1].SQL)
	}
}

func TestLoadFiltersByProfileAndDatabase(t *testing.T) {
	store := setHistoryPath(t)
	now := time.Now().UTC()

	entries := []Entry{
		{Profile: "dev", Database: "myapp", SQL: "A", At: now},
		{Profile: "dev", Database: "test", SQL: "B", At: now},
		{Profile: "prod", Database: "myapp", SQL: "C", At: now},
		{Profile: "dev", Database: "myapp", SQL: "D", At: now.Add(time.Second)},
	}
	for _, e := range entries {
		if err := store.Append(e); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.Load("dev", "myapp")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 dev/myapp entries, got %d: %+v", len(got), got)
	}
	if got[0].SQL != "D" || got[1].SQL != "A" {
		t.Errorf("unexpected ordering: got[0]=%q got[1]=%q", got[0].SQL, got[1].SQL)
	}
}

func TestLoadMissingFileReturnsNil(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_HISTORY_FILE", filepath.Join(dir, "no-such.jsonl"))
	store, err := New()
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Load("dev", "myapp")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestLoadSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	contents := `{"profile":"dev","database":"myapp","sql":"SELECT 1","at":"2026-04-13T10:00:00Z","ok":true,"duration_ms":1}
not-json
{"this":"is missing required fields but parses"}
{"profile":"dev","database":"myapp","sql":"SELECT 2","at":"2026-04-13T10:01:00Z","ok":true,"duration_ms":2}
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RDQ_HISTORY_FILE", path)
	store, err := New()
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Load("dev", "myapp")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 valid entries (malformed line skipped), got %d", len(got))
	}
	if got[0].SQL != "SELECT 2" || got[1].SQL != "SELECT 1" {
		t.Errorf("unexpected entries: %+v", got)
	}
}

func TestAppendCreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested", "subdir", "history.jsonl")
	t.Setenv("RDQ_HISTORY_FILE", nested)
	store, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(Entry{Profile: "dev", Database: "myapp", SQL: "SELECT 1"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("expected file at %s, got %v", nested, err)
	}
}

func TestAppendEntryWithError(t *testing.T) {
	store := setHistoryPath(t)
	if err := store.Append(Entry{
		Profile: "dev", Database: "myapp",
		SQL: "BAD SQL", At: time.Now().UTC(),
		Ok: false, DurationMS: 5, ErrorMsg: "syntax error",
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Load("dev", "myapp")
	if len(got) != 1 || got[0].Ok || got[0].ErrorMsg != "syntax error" {
		t.Errorf("error entry not persisted correctly: %+v", got)
	}
}

func TestSetFavoriteRoundTrip(t *testing.T) {
	store := setHistoryPath(t)
	at1 := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	at2 := time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)

	if err := store.Append(Entry{Profile: "dev", Database: "myapp", SQL: "SELECT 1", At: at1, Ok: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(Entry{Profile: "dev", Database: "myapp", SQL: "SELECT 2", At: at2, Ok: true}); err != nil {
		t.Fatal(err)
	}

	if err := store.SetFavorite(at1, true); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}

	got, err := store.Load("dev", "myapp")
	if err != nil {
		t.Fatal(err)
	}
	// Most-recent-first ordering: at2 then at1.
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Favorite {
		t.Errorf("at2 should not be favorited: %+v", got[0])
	}
	if !got[1].Favorite {
		t.Errorf("at1 should be favorited: %+v", got[1])
	}

	// Toggling off again restores the original state.
	if err := store.SetFavorite(at1, false); err != nil {
		t.Fatalf("SetFavorite off: %v", err)
	}
	got, _ = store.Load("dev", "myapp")
	if got[1].Favorite {
		t.Errorf("at1 should no longer be favorited: %+v", got[1])
	}
}

func TestSetFavoriteNoMatchIsNoOp(t *testing.T) {
	store := setHistoryPath(t)
	if err := store.Append(Entry{Profile: "dev", Database: "myapp", SQL: "SELECT 1", At: time.Now().UTC(), Ok: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetFavorite(time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), true); err != nil {
		t.Errorf("expected nil for non-matching timestamp, got %v", err)
	}
}

func TestSetFavoriteOnEmptyFile(t *testing.T) {
	store := setHistoryPath(t)
	if err := store.SetFavorite(time.Now(), true); err != nil {
		t.Errorf("SetFavorite on empty store should be no-op, got %v", err)
	}
}
