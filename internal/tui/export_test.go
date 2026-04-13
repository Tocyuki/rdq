package tui

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFormatCSVCell(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil is empty", nil, ""},
		{"string", "alice", "alice"},
		{"int64", int64(42), "42"},
		{"float64", 3.14, "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"blob base64", []byte("hello"), "aGVsbG8="},
		{"empty array", []any{}, "[]"},
		{"int array", []any{int64(1), int64(2)}, "[1, 2]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatCSVCell(tc.in)
			if got != tc.want {
				t.Errorf("formatCSVCell(%#v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWriteCSVRoundTrip(t *testing.T) {
	r := &queryResult{
		Columns: []string{"id", "name", "active", "score", "missing"},
		Rows: [][]any{
			{int64(1), "alice", true, 3.14, nil},
			{int64(2), "bob, jr", false, 2.5, nil},
			{int64(3), "with\nnewline", true, 0.0, nil},
		},
	}

	var buf bytes.Buffer
	if err := r.writeCSV(&buf); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("csv read: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("expected 4 records (header + 3 rows), got %d", len(records))
	}

	wantHeader := []string{"id", "name", "active", "score", "missing"}
	if !reflect.DeepEqual(records[0], wantHeader) {
		t.Errorf("header = %v, want %v", records[0], wantHeader)
	}

	wantRow1 := []string{"1", "alice", "true", "3.14", ""}
	if !reflect.DeepEqual(records[1], wantRow1) {
		t.Errorf("row 1 = %v, want %v", records[1], wantRow1)
	}

	// Row 2: "bob, jr" must be quoted automatically by encoding/csv
	if records[2][1] != "bob, jr" {
		t.Errorf("expected quoted comma to round-trip, got %q", records[2][1])
	}

	// Row 3: newline must round-trip through quoting
	if records[3][1] != "with\nnewline" {
		t.Errorf("expected newline to round-trip, got %q", records[3][1])
	}
}

func TestWriteCSVNilResult(t *testing.T) {
	var r *queryResult
	var buf bytes.Buffer
	if err := r.writeCSV(&buf); err != nil {
		t.Errorf("expected nil error for nil result, got %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

func TestExportCSVCreatesFile(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	r := &queryResult{
		Columns: []string{"id"},
		Rows:    [][]any{{int64(1)}, {int64(2)}},
	}
	path, err := r.exportCSV()
	if err != nil {
		t.Fatalf("exportCSV: %v", err)
	}
	// On macOS the temp dir is a symlink (/var → /private/var) so compare
	// the resolved paths rather than literal strings.
	gotDir, _ := filepath.EvalSymlinks(filepath.Dir(path))
	wantDir, _ := filepath.EvalSymlinks(dir)
	if gotDir != wantDir {
		t.Errorf("expected file in %s, got %s", wantDir, gotDir)
	}
	if !strings.HasPrefix(filepath.Base(path), "rdq-") || !strings.HasSuffix(path, ".csv") {
		t.Errorf("unexpected filename: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "id\n1\n2\n") {
		t.Errorf("unexpected file contents:\n%s", data)
	}
}

func TestExportCSVNoResult(t *testing.T) {
	r := &queryResult{}
	if _, err := r.exportCSV(); err == nil {
		t.Error("expected error for empty result, got nil")
	}
}

func TestExportCSVCollisionAddsSuffix(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	r := &queryResult{Columns: []string{"id"}, Rows: [][]any{{int64(1)}}}
	first, err := r.exportCSV()
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.exportCSV()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Errorf("expected unique filenames, both = %s", first)
	}
}
