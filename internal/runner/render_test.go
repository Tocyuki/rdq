package runner

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderTable_SelectShape(t *testing.T) {
	r := &Result{
		Columns: []string{"id", "name"},
		Rows: [][]any{
			{int64(1), "alice"},
			{int64(2), "bob"},
		},
		Updated: -1,
	}
	var buf bytes.Buffer
	if err := RenderTable(&buf, r); err != nil {
		t.Fatalf("RenderTable: %v", err)
	}
	want := "+----+-------+\n" +
		"| id | name  |\n" +
		"+----+-------+\n" +
		"|  1 | alice |\n" +
		"|  2 | bob   |\n" +
		"+----+-------+\n" +
		"(2 rows)\n"
	if got := buf.String(); got != want {
		t.Errorf("unexpected output:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderTable_SingleRowFooter(t *testing.T) {
	r := &Result{
		Columns: []string{"id"},
		Rows:    [][]any{{int64(42)}},
		Updated: -1,
	}
	var buf bytes.Buffer
	if err := RenderTable(&buf, r); err != nil {
		t.Fatalf("RenderTable: %v", err)
	}
	if !strings.HasSuffix(buf.String(), "(1 row)\n") {
		t.Errorf("expected (1 row) footer for singular, got:\n%s", buf.String())
	}
}

func TestRenderTable_ZeroRows(t *testing.T) {
	r := &Result{
		Columns: []string{"id", "name"},
		Rows:    nil,
		Updated: -1,
	}
	var buf bytes.Buffer
	if err := RenderTable(&buf, r); err != nil {
		t.Fatalf("RenderTable: %v", err)
	}
	want := "+----+------+\n" +
		"| id | name |\n" +
		"+----+------+\n" +
		"+----+------+\n" +
		"(0 rows)\n"
	if got := buf.String(); got != want {
		t.Errorf("unexpected output:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderTable_NullCell(t *testing.T) {
	r := &Result{
		Columns: []string{"id", "email"},
		Rows: [][]any{
			{int64(1), nil},
			{int64(2), "bob@example.com"},
		},
		Updated: -1,
	}
	var buf bytes.Buffer
	if err := RenderTable(&buf, r); err != nil {
		t.Fatalf("RenderTable: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "NULL") {
		t.Errorf("expected NULL to appear in output, got:\n%s", out)
	}
}

func TestRenderTable_NumericRightAlign(t *testing.T) {
	r := &Result{
		Columns: []string{"n"},
		Rows: [][]any{
			{int64(1)},
			{int64(100)},
		},
		Updated: -1,
	}
	var buf bytes.Buffer
	if err := RenderTable(&buf, r); err != nil {
		t.Fatalf("RenderTable: %v", err)
	}
	// Right-aligned: the "1" lines up with the rightmost "0" of "100".
	out := buf.String()
	if !strings.Contains(out, "|   1 |\n") {
		t.Errorf("expected right-aligned single digit, got:\n%s", out)
	}
	if !strings.Contains(out, "| 100 |\n") {
		t.Errorf("expected 100 to fill width, got:\n%s", out)
	}
}

func TestRenderTable_MixedColumnLeftAlign(t *testing.T) {
	r := &Result{
		Columns: []string{"val"},
		Rows: [][]any{
			{int64(1)},
			{"two"},
		},
		Updated: -1,
	}
	var buf bytes.Buffer
	if err := RenderTable(&buf, r); err != nil {
		t.Fatalf("RenderTable: %v", err)
	}
	// Mixed column → left-aligned → "1  " with trailing padding.
	out := buf.String()
	if !strings.Contains(out, "| 1   |\n") {
		t.Errorf("expected left-aligned 1 with padding, got:\n%s", out)
	}
	if !strings.Contains(out, "| two |\n") {
		t.Errorf("expected two to fill width, got:\n%s", out)
	}
}

func TestRenderTable_TruncatesLongCell(t *testing.T) {
	long := strings.Repeat("x", ColumnWidthCap+20)
	r := &Result{
		Columns: []string{"data"},
		Rows:    [][]any{{long}},
		Updated: -1,
	}
	var buf bytes.Buffer
	if err := RenderTable(&buf, r); err != nil {
		t.Fatalf("RenderTable: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "…") {
		t.Errorf("expected ellipsis for truncated cell, got:\n%s", out)
	}
}

func TestRenderTable_NilResult(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderTable(&buf, nil); err != nil {
		t.Fatalf("RenderTable(nil): %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output for nil, got %q", buf.String())
	}
}

func TestRenderTable_EmptyColumns(t *testing.T) {
	r := &Result{Columns: nil, Rows: nil, Updated: 0}
	var buf bytes.Buffer
	if err := RenderTable(&buf, r); err != nil {
		t.Fatalf("RenderTable: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output for column-less result, got %q", buf.String())
	}
}

func TestRenderTable_WriteShapeNoFooter(t *testing.T) {
	// Updated >= 0 indicates an UPDATE/INSERT/DELETE-like result. Such
	// queries typically have no columns, but if they do (e.g. RETURNING),
	// we skip the "(N rows)" footer so the caller can emit
	// "(N rows affected)" to stderr instead.
	r := &Result{
		Columns: []string{"id"},
		Rows:    [][]any{{int64(1)}},
		Updated: 1,
	}
	var buf bytes.Buffer
	if err := RenderTable(&buf, r); err != nil {
		t.Fatalf("RenderTable: %v", err)
	}
	if strings.Contains(buf.String(), "row") {
		t.Errorf("write-shape result should not emit (N rows) footer, got:\n%s", buf.String())
	}
}

func TestBuildSeparatorRow(t *testing.T) {
	got := buildSeparatorRow([]int{3, 5, 1})
	want := "+-----+-------+---+"
	if got != want {
		t.Errorf("buildSeparatorRow = %q, want %q", got, want)
	}
}

func TestDetectNumericColumns(t *testing.T) {
	cols := []string{"id", "name", "score", "flag", "mixed", "empty"}
	rows := [][]any{
		{int64(1), "alice", 3.14, true, int64(1), nil},
		{int64(2), "bob", 2.5, false, "two", nil},
	}
	got := detectNumericColumns(cols, rows)
	want := []bool{true, false, true, false, false, false}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column %q (idx %d): numeric=%v, want %v", cols[i], i, got[i], want[i])
		}
	}
}
