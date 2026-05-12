package aictx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	c := &Context{
		Cluster:  "arn:aws:rds:ap-northeast-1:111:cluster:my",
		Database: "myapp",
		Content:  "active user = last_login_at within 30 days\n注文テーブルは orders.deleted_at IS NULL のみ集計対象",
	}
	if err := Save(c); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(c.Cluster, c.Database)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected context, got nil")
	}
	if loaded.Database != c.Database {
		t.Errorf("database mismatch: got %q, want %q", loaded.Database, c.Database)
	}
	if loaded.Content != c.Content {
		t.Errorf("content mismatch: got %q, want %q", loaded.Content, c.Content)
	}
	if loaded.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestLoadMissingFileReturnsNil(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	got, err := Load("arn:foo", "bar")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil context, got %+v", got)
	}
}

func TestLoadContentTrimsAndReturnsEmptyForMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	got, err := LoadContent("arn:foo", "bar")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for missing file, got %q", got)
	}

	if err := Save(&Context{Cluster: "arn:foo", Database: "bar", Content: "  hello\n\n"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err = LoadContent("arn:foo", "bar")
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	if got != "hello" {
		t.Errorf("expected trimmed content %q, got %q", "hello", got)
	}
}

func TestSaveRejectsEmptyContent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	cases := []string{"", "   ", "\n\t\n"}
	for _, content := range cases {
		err := Save(&Context{Cluster: "c", Database: "d", Content: content})
		if err == nil {
			t.Errorf("expected error for empty content %q, got nil", content)
		}
	}
}

func TestSaveRejectsMissingKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	if err := Save(&Context{Database: "d", Content: "x"}); err == nil {
		t.Error("expected error for missing cluster")
	}
	if err := Save(&Context{Cluster: "c", Content: "x"}); err == nil {
		t.Error("expected error for missing database")
	}
}

func TestSaveRejectsTooLargeContent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	big := strings.Repeat("x", MaxContentBytes+1)
	if err := Save(&Context{Cluster: "c", Database: "d", Content: big}); err == nil {
		t.Error("expected error for oversized content")
	}
}

func TestSaveLimitIsBytesNotRunes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	// 全角ひらがな: 1 rune = 3 UTF-8 bytes. Use enough runes that the byte
	// count crosses MaxContentBytes while the rune count alone wouldn't.
	// Catches a future regression that mistakenly compares runes.
	runeCount := MaxContentBytes/3 + 1
	multibyte := strings.Repeat("あ", runeCount)
	if got := len(multibyte); got <= MaxContentBytes {
		t.Fatalf("test setup: byte length %d should exceed cap %d", got, MaxContentBytes)
	}
	if err := Save(&Context{Cluster: "c", Database: "d", Content: multibyte}); err == nil {
		t.Error("expected oversized error for multi-byte content past byte cap")
	}
}

func TestSaveTrimsAndStampsUpdatedAt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	c := &Context{Cluster: "c", Database: "d", Content: "  hello  \n"}
	before := time.Now().Add(-time.Second)
	if err := Save(c); err != nil {
		t.Fatalf("save: %v", err)
	}
	if c.Content != "hello" {
		t.Errorf("expected Save to trim Content in place, got %q", c.Content)
	}
	if !c.UpdatedAt.After(before) {
		t.Errorf("expected UpdatedAt to be stamped after %v, got %v", before, c.UpdatedAt)
	}
}

func TestDeleteRemovesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	c := &Context{Cluster: "c", Database: "d", Content: "x"}
	if err := Save(c); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := Delete(c.Cluster, c.Database); err != nil {
		t.Fatalf("delete: %v", err)
	}
	loaded, err := Load(c.Cluster, c.Database)
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil after delete, got %+v", loaded)
	}
}

func TestDeleteMissingFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	if err := Delete("c", "d"); err != nil {
		t.Errorf("delete on missing file should be nil, got %v", err)
	}
}

func TestLoadCorruptJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	path, err := contextPath("c", "d")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load("c", "d"); err == nil {
		t.Error("expected parse error, got nil")
	}
}

func TestContextPathHonorsEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_AICTX_DIR", dir)

	path, err := contextPath("c", "d")
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
