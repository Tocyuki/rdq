package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_SCHEMA_DIR", dir)

	s := &Snapshot{
		Version:   snapshotVersion,
		Cluster:   "arn:aws:rds:ap-northeast-1:111:cluster:my",
		Database:  "myapp",
		FetchedAt: time.Now().UTC().Truncate(time.Second),
		Columns: []Column{
			{Schema: "myapp", Table: "users", Name: "id", Type: "bigint"},
			{Schema: "myapp", Table: "users", Name: "name", Type: "varchar"},
			{Schema: "myapp", Table: "orders", Name: "id", Type: "bigint"},
		},
	}
	if err := SaveCache(s); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadCache(s.Cluster, s.Database)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected cached snapshot, got nil")
	}
	if loaded.Database != s.Database {
		t.Errorf("database mismatch: got %q, want %q", loaded.Database, s.Database)
	}
	if len(loaded.Columns) != len(s.Columns) {
		t.Errorf("column count: got %d, want %d", len(loaded.Columns), len(s.Columns))
	}
}

func TestLoadCacheMissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_SCHEMA_DIR", dir)
	got, err := LoadCache("arn:foo", "bar")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil snapshot, got %+v", got)
	}
}

func TestCachePathHonorsEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_SCHEMA_DIR", dir)
	path, err := cachePath("arn:foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("expected dir %s, got %s", dir, filepath.Dir(path))
	}
	if !strings.HasSuffix(path, ".json") {
		t.Errorf("expected .json extension, got %s", path)
	}
}

func TestToPromptGroupsByTable(t *testing.T) {
	s := &Snapshot{
		Database: "myapp",
		Columns: []Column{
			{Schema: "myapp", Table: "users", Name: "id", Type: "bigint"},
			{Schema: "myapp", Table: "users", Name: "name", Type: "varchar"},
			{Schema: "myapp", Table: "users", Name: "email", Type: "varchar"},
			{Schema: "myapp", Table: "orders", Name: "id", Type: "bigint"},
			{Schema: "myapp", Table: "orders", Name: "user_id", Type: "bigint"},
		},
	}
	out := s.ToPrompt()

	if !strings.Contains(out, "TABLE users (") {
		t.Errorf("expected users table heading, got:\n%s", out)
	}
	if !strings.Contains(out, "TABLE orders (") {
		t.Errorf("expected orders table heading, got:\n%s", out)
	}
	if !strings.Contains(out, "  id bigint,") {
		t.Errorf("expected formatted column line, got:\n%s", out)
	}
	if !strings.Contains(out, "  email varchar\n") {
		t.Errorf("expected last column without trailing comma, got:\n%s", out)
	}

	// orders should appear before users alphabetically.
	if strings.Index(out, "TABLE orders") > strings.Index(out, "TABLE users") {
		t.Errorf("expected orders before users:\n%s", out)
	}
}

func TestToPromptPrefixesNonDefaultSchema(t *testing.T) {
	s := &Snapshot{
		Database: "myapp",
		Columns: []Column{
			{Schema: "audit", Table: "log", Name: "id", Type: "bigint"},
		},
	}
	out := s.ToPrompt()
	if !strings.Contains(out, "TABLE audit.log") {
		t.Errorf("expected audit.log prefix, got:\n%s", out)
	}
}

func TestToPromptEmpty(t *testing.T) {
	s := &Snapshot{}
	out := s.ToPrompt()
	if out != "(no schema available)" {
		t.Errorf("expected fallback message, got %q", out)
	}
}

// TestLoadCacheTreatsOldVersionAsMiss: caches written before primary-key
// support (no version field) must be refetched, otherwise the GUI preview
// would fall back to ordering by every column on tables that have a PK.
func TestLoadCacheTreatsOldVersionAsMiss(t *testing.T) {
	t.Setenv("RDQ_SCHEMA_DIR", t.TempDir())
	cluster, database := "arn:c", "db"
	path, err := cachePath(cluster, database)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := `{"cluster":"arn:c","database":"db","fetched_at":"2026-01-01T00:00:00Z","columns":[{"schema":"public","table":"t","name":"id","type":"integer"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadCache(cluster, database); err != nil || got != nil {
		t.Fatalf("legacy cache should be a miss, got %+v, %v", got, err)
	}

	// A current-version cache is served even when the PK lookup had
	// failed (PrimaryKeysKnown=false), so it is not refetched every time.
	current := &Snapshot{Version: snapshotVersion, Cluster: cluster, Database: database,
		Columns: []Column{{Schema: "public", Table: "t", Name: "id", Type: "integer"}}}
	if err := SaveCache(current); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadCache(cluster, database); err != nil || got == nil {
		t.Fatalf("current cache should load, got %+v, %v", got, err)
	}
}

func TestFetchStampsSnapshotVersion(t *testing.T) {
	f := &fakeExec{columns: manyColumns(1)}
	snap, err := fetchWith(t.Context(), f.exec, "arn:c", "db")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version != snapshotVersion {
		t.Fatalf("Version = %d, want %d", snap.Version, snapshotVersion)
	}
}
