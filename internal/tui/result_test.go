package tui

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata/types"
)

func TestFieldToString(t *testing.T) {
	cases := []struct {
		name string
		in   types.Field
		want string
	}{
		{"nil", nil, "NULL"},
		{"isNull", &types.FieldMemberIsNull{Value: true}, "NULL"},
		{"string", &types.FieldMemberStringValue{Value: "hello"}, "hello"},
		{"long", &types.FieldMemberLongValue{Value: 42}, "42"},
		{"long zero", &types.FieldMemberLongValue{Value: 0}, "0"},
		{"long negative", &types.FieldMemberLongValue{Value: -7}, "-7"},
		{"double", &types.FieldMemberDoubleValue{Value: 3.14}, "3.14"},
		{"double whole", &types.FieldMemberDoubleValue{Value: 2.0}, "2"},
		{"bool true", &types.FieldMemberBooleanValue{Value: true}, "true"},
		{"bool false", &types.FieldMemberBooleanValue{Value: false}, "false"},
		{"blob", &types.FieldMemberBlobValue{Value: []byte("xyz")}, "<blob 3 bytes>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fieldToString(tc.in)
			if got != tc.want {
				t.Errorf("fieldToString = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestArrayToString(t *testing.T) {
	cases := []struct {
		name string
		in   types.ArrayValue
		want string
	}{
		{"strings", &types.ArrayValueMemberStringValues{Value: []string{"a", "b"}}, `["a", "b"]`},
		{"longs", &types.ArrayValueMemberLongValues{Value: []int64{1, 2, 3}}, "[1, 2, 3]"},
		{"doubles", &types.ArrayValueMemberDoubleValues{Value: []float64{1.5, 2.5}}, "[1.5, 2.5]"},
		{"bools", &types.ArrayValueMemberBooleanValues{Value: []bool{true, false}}, "[true, false]"},
		{"empty", &types.ArrayValueMemberStringValues{Value: nil}, "[]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := arrayToString(tc.in)
			if got != tc.want {
				t.Errorf("arrayToString = %q, want %q", got, tc.want)
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
		NumberOfRecordsUpdated: 0,
	}

	got := convertResult(out)
	wantCols := []string{"id", "name", "active"}
	if !reflect.DeepEqual(got.Columns, wantCols) {
		t.Errorf("columns = %v, want %v", got.Columns, wantCols)
	}
	wantRows := [][]string{
		{"1", "alice", "true"},
		{"2", "NULL", "false"},
	}
	if !reflect.DeepEqual(got.Rows, wantRows) {
		t.Errorf("rows = %v, want %v", got.Rows, wantRows)
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
	got := convertResult(out)
	if len(got.Rows) != 1 || len(got.Rows[0]) != 3 {
		t.Fatalf("expected 1 row of 3 cells, got %+v", got.Rows)
	}
	if got.Rows[0][2] != nullDisplay {
		t.Errorf("expected NULL padding for missing cell, got %q", got.Rows[0][2])
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
	got := convertResult(out)
	if len(got.Rows) != 1 || len(got.Rows[0]) != 1 {
		t.Fatalf("expected 1 row truncated to 1 cell, got %+v", got.Rows)
	}
	if got.Rows[0][0] != "1" {
		t.Errorf("unexpected first cell: %q", got.Rows[0][0])
	}
}

func TestConvertResultNil(t *testing.T) {
	got := convertResult(nil)
	if got == nil {
		t.Fatal("expected empty queryResult, got nil")
	}
	if len(got.Columns) != 0 || len(got.Rows) != 0 {
		t.Errorf("expected empty result, got %+v", got)
	}
}

func TestColumnWidths(t *testing.T) {
	cols := []string{"id", "longheader", "x"}
	rows := [][]string{
		{"1", "short", "a"},
		{"99", "this is a much longer value", "bb"},
	}
	got := columnWidths(cols, rows)
	want := []int{2, 27, 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("widths = %v, want %v", got, want)
	}
}

func TestColumnWidthsCap(t *testing.T) {
	cols := []string{"id"}
	rows := [][]string{
		{"this string is way longer than the forty character cap"},
	}
	got := columnWidths(cols, rows)
	if got[0] != columnWidthCap {
		t.Errorf("width capped to %d, got %d", columnWidthCap, got[0])
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
		got := truncate(tc.s, tc.width)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.s, tc.width, got, tc.want)
		}
	}
}

func TestQueryResultToJSON(t *testing.T) {
	r := &queryResult{
		Columns: []string{"id", "name"},
		Rows: [][]string{
			{"1", "alice"},
			{"2", "bob"},
		},
	}
	out := r.toJSON()

	var decoded []map[string]string
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(decoded))
	}
	if decoded[0]["name"] != "alice" || decoded[1]["id"] != "2" {
		t.Errorf("unexpected decoded JSON: %+v", decoded)
	}
}
