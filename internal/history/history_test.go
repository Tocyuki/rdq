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
