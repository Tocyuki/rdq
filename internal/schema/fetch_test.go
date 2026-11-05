package schema

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata/types"
)

var limitOffsetRe = regexp.MustCompile(`LIMIT (\d+) OFFSET (\d+)$`)

func str(s string) types.Field { return &types.FieldMemberStringValue{Value: s} }
func long(n int64) types.Field { return &types.FieldMemberLongValue{Value: n} }

// fakeExec serves rows for each query family with real LIMIT/OFFSET
// semantics and records every statement it saw.
type fakeExec struct {
	columns   [][]types.Field
	pks       [][]types.Field
	failQuery func(sql string) bool
	seen      []string
}

func (f *fakeExec) exec(_ context.Context, sql string) (*rdsdata.ExecuteStatementOutput, error) {
	f.seen = append(f.seen, sql)
	if f.failQuery != nil && f.failQuery(sql) {
		return nil, errors.New("query rejected")
	}
	m := limitOffsetRe.FindStringSubmatch(sql)
	if m == nil {
		return nil, fmt.Errorf("query is not paged: %q", sql)
	}
	limit, _ := strconv.Atoi(m[1])
	offset, _ := strconv.Atoi(m[2])
	rows := f.columns
	if strings.Contains(sql, "KEY_COLUMN_USAGE") {
		rows = f.pks
	}
	if offset > len(rows) {
		offset = len(rows)
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return &rdsdata.ExecuteStatementOutput{Records: rows[offset:end]}, nil
}

func manyColumns(n int) [][]types.Field {
	rows := make([][]types.Field, n)
	for i := range rows {
		rows[i] = []types.Field{str("public"), str(fmt.Sprintf("t%d", i/10)), str(fmt.Sprintf("c%d", i%10)), str("text")}
	}
	return rows
}

func TestFetchPagesThroughLargeSchemas(t *testing.T) {
	const total = fetchPageSize*2 + 7
	f := &fakeExec{columns: manyColumns(total)}
	snap, err := fetchWith(context.Background(), f.exec, "arn:c", "db")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Columns) != total {
		t.Fatalf("got %d columns, want %d", len(snap.Columns), total)
	}
	columnQueries := 0
	for _, q := range f.seen {
		if strings.Contains(q, "information_schema.COLUMNS") {
			columnQueries++
		}
	}
	if columnQueries != 3 {
		t.Errorf("column queries = %d, want 3 pages", columnQueries)
	}
}

func TestFetchExactMultipleOfPageSize(t *testing.T) {
	f := &fakeExec{columns: manyColumns(fetchPageSize)}
	snap, err := fetchWith(context.Background(), f.exec, "arn:c", "db")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Columns) != fetchPageSize {
		t.Fatalf("got %d columns, want %d", len(snap.Columns), fetchPageSize)
	}
}

func TestFetchFallsBackWhenFilteredQueryFails(t *testing.T) {
	f := &fakeExec{
		columns:   manyColumns(3),
		failQuery: func(sql string) bool { return strings.Contains(sql, "SCHEMA_OWNER") },
	}
	snap, err := fetchWith(context.Background(), f.exec, "arn:c", "db")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Columns) != 3 {
		t.Fatalf("fallback query should supply columns, got %d", len(snap.Columns))
	}
}

func TestFetchAppliesPrimaryKeys(t *testing.T) {
	f := &fakeExec{
		columns: [][]types.Field{
			{str("public"), str("orders"), str("tenant_id"), str("integer")},
			{str("public"), str("orders"), str("id"), str("integer")},
			{str("public"), str("orders"), str("note"), str("text")},
		},
		pks: [][]types.Field{
			{str("public"), str("orders"), str("tenant_id"), long(1)},
			{str("public"), str("orders"), str("id"), str("2")}, // cardinal_number as string
		},
	}
	snap, err := fetchWith(context.Background(), f.exec, "arn:c", "db")
	if err != nil {
		t.Fatal(err)
	}
	if !snap.PrimaryKeysKnown {
		t.Fatal("PrimaryKeysKnown should be true when the PK query succeeds")
	}
	want := map[string]int{"tenant_id": 1, "id": 2, "note": 0}
	for _, c := range snap.Columns {
		if c.PrimaryKey != want[c.Name] {
			t.Errorf("%s.PrimaryKey = %d, want %d", c.Name, c.PrimaryKey, want[c.Name])
		}
	}
}

func TestFetchPrimaryKeyFailureIsNonFatal(t *testing.T) {
	f := &fakeExec{
		columns:   manyColumns(2),
		failQuery: func(sql string) bool { return strings.Contains(sql, "KEY_COLUMN_USAGE") },
	}
	snap, err := fetchWith(context.Background(), f.exec, "arn:c", "db")
	if err != nil {
		t.Fatalf("PK lookup failure must not fail Fetch: %v", err)
	}
	if snap.PrimaryKeysKnown || len(snap.Columns) != 2 {
		t.Fatalf("unexpected snapshot: known=%v cols=%d", snap.PrimaryKeysKnown, len(snap.Columns))
	}
}

func TestFetchBothColumnQueriesFail(t *testing.T) {
	f := &fakeExec{failQuery: func(string) bool { return true }}
	if _, err := fetchWith(context.Background(), f.exec, "arn:c", "db"); err == nil {
		t.Fatal("expected an error when both column queries fail")
	}
}
