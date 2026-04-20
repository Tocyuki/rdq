package runner

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata/types"
)

func TestFieldValue(t *testing.T) {
	cases := []struct {
		name string
		in   types.Field
		want any
	}{
		{"nil", nil, nil},
		{"isNull", &types.FieldMemberIsNull{Value: true}, nil},
		{"string", &types.FieldMemberStringValue{Value: "hello"}, "hello"},
		{"long", &types.FieldMemberLongValue{Value: 42}, int64(42)},
		{"long zero", &types.FieldMemberLongValue{Value: 0}, int64(0)},
		{"long negative", &types.FieldMemberLongValue{Value: -7}, int64(-7)},
		{"double", &types.FieldMemberDoubleValue{Value: 3.14}, 3.14},
		{"double whole", &types.FieldMemberDoubleValue{Value: 2.0}, 2.0},
		{"bool true", &types.FieldMemberBooleanValue{Value: true}, true},
		{"bool false", &types.FieldMemberBooleanValue{Value: false}, false},
		{"blob", &types.FieldMemberBlobValue{Value: []byte("xyz")}, []byte("xyz")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fieldValue(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("fieldValue = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestFormatCell(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, "NULL"},
		{"string", "hello", "hello"},
		{"int64", int64(42), "42"},
		{"float64", 3.14, "3.14"},
		{"float64 whole", 2.0, "2"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"blob", []byte("xyz"), "<blob 3 bytes>"},
		{"empty array", []any{}, "[]"},
		{"string array", []any{"a", "b"}, `["a", "b"]`},
		{"int array", []any{int64(1), int64(2), int64(3)}, "[1, 2, 3]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatCell(tc.in)
			if got != tc.want {
				t.Errorf("FormatCell(%#v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestArrayValue(t *testing.T) {
	cases := []struct {
		name string
		in   types.ArrayValue
		want []any
	}{
		{"strings", &types.ArrayValueMemberStringValues{Value: []string{"a", "b"}}, []any{"a", "b"}},
		{"longs", &types.ArrayValueMemberLongValues{Value: []int64{1, 2}}, []any{int64(1), int64(2)}},
		{"doubles", &types.ArrayValueMemberDoubleValues{Value: []float64{1.5, 2.5}}, []any{1.5, 2.5}},
		{"bools", &types.ArrayValueMemberBooleanValues{Value: []bool{true, false}}, []any{true, false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := arrayValue(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("arrayValue = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestConvertResult(t *testing.T) {
	out := &rdsdata.ExecuteStatementOutput{
		ColumnMetadata: []types.ColumnMetadata{
			{Name: aws.String("id")},
			{Name: aws.String("name")},
			{Name: aws.String("active")},
		},
		Records: [][]types.Field{
			{
				&types.FieldMemberLongValue{Value: 1},
				&types.FieldMemberStringValue{Value: "alice"},
				&types.FieldMemberBooleanValue{Value: true},
			},
			{
				&types.FieldMemberLongValue{Value: 2},
				&types.FieldMemberIsNull{Value: true},
				&types.FieldMemberBooleanValue{Value: false},
			},
		},
	}

	got := ConvertResult(out)
	wantCols := []string{"id", "name", "active"}
	if !reflect.DeepEqual(got.Columns, wantCols) {
		t.Errorf("columns = %v, want %v", got.Columns, wantCols)
	}
	wantRows := [][]any{
		{int64(1), "alice", true},
		{int64(2), nil, false},
	}
	if !reflect.DeepEqual(got.Rows, wantRows) {
		t.Errorf("rows = %#v, want %#v", got.Rows, wantRows)
	}
}

func TestConvertResultRowShorterThanColumns(t *testing.T) {
	out := &rdsdata.ExecuteStatementOutput{
		ColumnMetadata: []types.ColumnMetadata{
			{Name: aws.String("id")},
			{Name: aws.String("name")},
			{Name: aws.String("email")},
		},
		Records: [][]types.Field{
			{
				&types.FieldMemberLongValue{Value: 1},
				&types.FieldMemberStringValue{Value: "alice"},
			},
		},
	}
	got := ConvertResult(out)
	if len(got.Rows) != 1 || len(got.Rows[0]) != 3 {
		t.Fatalf("expected 1 row of 3 cells, got %+v", got.Rows)
	}
	if got.Rows[0][2] != nil {
		t.Errorf("expected nil padding for missing cell, got %#v", got.Rows[0][2])
	}
}

func TestConvertResultRowLongerThanColumns(t *testing.T) {
	out := &rdsdata.ExecuteStatementOutput{
		ColumnMetadata: []types.ColumnMetadata{
			{Name: aws.String("id")},
		},
		Records: [][]types.Field{
			{
				&types.FieldMemberLongValue{Value: 1},
				&types.FieldMemberStringValue{Value: "extra"},
				&types.FieldMemberStringValue{Value: "more"},
			},
		},
	}
	got := ConvertResult(out)
	if len(got.Rows) != 1 || len(got.Rows[0]) != 1 {
		t.Fatalf("expected 1 row truncated to 1 cell, got %+v", got.Rows)
	}
	if got.Rows[0][0] != int64(1) {
		t.Errorf("unexpected first cell: %#v", got.Rows[0][0])
	}
}

func TestConvertResultNil(t *testing.T) {
	got := ConvertResult(nil)
	if got == nil {
		t.Fatal("expected empty Result, got nil")
	}
	if len(got.Columns) != 0 || len(got.Rows) != 0 {
		t.Errorf("expected empty result, got %+v", got)
	}
}

func TestColumnWidths(t *testing.T) {
	cols := []string{"id", "longheader", "x"}
	rows := [][]any{
		{int64(1), "short", "a"},
		{int64(99), "this is a much longer value", "bb"},
	}
	got := ColumnWidths(cols, rows)
	want := []int{2, 27, 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("widths = %v, want %v", got, want)
	}
}

func TestColumnWidthsCap(t *testing.T) {
	cols := []string{"id"}
	rows := [][]any{
		{"this string is way longer than the forty character cap"},
	}
	got := ColumnWidths(cols, rows)
	if got[0] != ColumnWidthCap {
		t.Errorf("width capped to %d, got %d", ColumnWidthCap, got[0])
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		s     string
		width int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
	}
	for _, tc := range cases {
		got := Truncate(tc.s, tc.width)
		if got != tc.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tc.s, tc.width, got, tc.want)
		}
	}
}

func TestResultToJSONPreservesTypes(t *testing.T) {
	r := &Result{
		Columns: []string{"id", "name", "active", "score", "missing"},
		Rows: [][]any{
			{int64(1), "alice", true, 3.14, nil},
			{int64(2), "bob", false, 2.5, nil},
		},
	}
	out := r.ToJSON()

	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(decoded))
	}

	first := decoded[0]
	// json.Unmarshal returns float64 for any JSON number.
	if id, ok := first["id"].(float64); !ok || id != 1 {
		t.Errorf("expected id=1 (number), got %#v", first["id"])
	}
	if name, ok := first["name"].(string); !ok || name != "alice" {
		t.Errorf("expected name=\"alice\", got %#v", first["name"])
	}
	if active, ok := first["active"].(bool); !ok || !active {
		t.Errorf("expected active=true, got %#v", first["active"])
	}
	if score, ok := first["score"].(float64); !ok || score != 3.14 {
		t.Errorf("expected score=3.14, got %#v", first["score"])
	}
	if first["missing"] != nil {
		t.Errorf("expected missing=null, got %#v", first["missing"])
	}
}

func TestRowJSONPreservesColumnOrder(t *testing.T) {
	r := &Result{
		Columns: []string{"zeta", "alpha", "mike"},
		Rows: [][]any{
			{int64(1), "first", true},
			{int64(2), "second", false},
		},
	}
	got, err := r.RowJSON(0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\"zeta\": 1") {
		t.Errorf("expected zeta first, got:\n%s", got)
	}
	zetaIdx := strings.Index(got, "\"zeta\"")
	alphaIdx := strings.Index(got, "\"alpha\"")
	mikeIdx := strings.Index(got, "\"mike\"")
	if zetaIdx < 0 || alphaIdx < 0 || mikeIdx < 0 {
		t.Fatalf("missing keys in output:\n%s", got)
	}
	if !(zetaIdx < alphaIdx && alphaIdx < mikeIdx) {
		t.Errorf("column order not preserved:\n%s", got)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, got)
	}
	if id, ok := decoded["zeta"].(float64); !ok || id != 1 {
		t.Errorf("expected zeta=1 (number), got %#v", decoded["zeta"])
	}
	if v, ok := decoded["alpha"].(string); !ok || v != "first" {
		t.Errorf("expected alpha=\"first\", got %#v", decoded["alpha"])
	}
}

func TestRowJSONHandlesNullsAndBlobs(t *testing.T) {
	r := &Result{
		Columns: []string{"id", "data", "missing"},
		Rows:    [][]any{{int64(7), []byte("hi"), nil}},
	}
	got, err := r.RowJSON(0)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, got)
	}
	if decoded["missing"] != nil {
		t.Errorf("expected missing=null, got %#v", decoded["missing"])
	}
	if v, _ := decoded["data"].(string); v != "aGk=" {
		t.Errorf("expected data base64, got %#v", decoded["data"])
	}
}

func TestRowJSONOutOfRange(t *testing.T) {
	r := &Result{Columns: []string{"id"}, Rows: [][]any{{int64(1)}}}
	cases := []int{-1, 1, 99}
	for _, idx := range cases {
		if _, err := r.RowJSON(idx); err == nil {
			t.Errorf("expected error for index %d, got nil", idx)
		}
	}
}

func TestResultToJSONBlobBase64(t *testing.T) {
	r := &Result{
		Columns: []string{"data"},
		Rows: [][]any{
			{[]byte("hello")},
		},
	}
	out := r.ToJSON()
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if got, _ := decoded[0]["data"].(string); got != "aGVsbG8=" {
		t.Errorf("expected base64 \"aGVsbG8=\", got %#v", decoded[0]["data"])
	}
}

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
	r := &Result{
		Columns: []string{"id", "name", "active", "score", "missing"},
		Rows: [][]any{
			{int64(1), "alice", true, 3.14, nil},
			{int64(2), "bob, jr", false, 2.5, nil},
			{int64(3), "with\nnewline", true, 0.0, nil},
		},
	}

	var buf bytes.Buffer
	if err := r.WriteCSV(&buf); err != nil {
		t.Fatalf("WriteCSV: %v", err)
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

	if records[2][1] != "bob, jr" {
		t.Errorf("expected quoted comma to round-trip, got %q", records[2][1])
	}

	if records[3][1] != "with\nnewline" {
		t.Errorf("expected newline to round-trip, got %q", records[3][1])
	}
}

func TestWriteCSVNilResult(t *testing.T) {
	var r *Result
	var buf bytes.Buffer
	if err := r.WriteCSV(&buf); err != nil {
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

	r := &Result{
		Columns: []string{"id"},
		Rows:    [][]any{{int64(1)}, {int64(2)}},
	}
	path, err := r.ExportCSV()
	if err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
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
	r := &Result{}
	if _, err := r.ExportCSV(); err == nil {
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

	r := &Result{Columns: []string{"id"}, Rows: [][]any{{int64(1)}}}
	first, err := r.ExportCSV()
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.ExportCSV()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Errorf("expected unique filenames, both = %s", first)
	}
}
