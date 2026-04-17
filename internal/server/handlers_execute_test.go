package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tocyuki/rdq/internal/history"
	"github.com/Tocyuki/rdq/internal/runner"
	"github.com/aws/aws-sdk-go-v2/aws"
)

func newTestExecuteHandlers(t *testing.T) (*executeHandlers, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("RDQ_HISTORY_FILE", filepath.Join(dir, "history.jsonl"))

	c := newAWSCache()
	c.loader = func(_ context.Context, _ string) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	hist, err := history.New()
	if err != nil {
		t.Fatal(err)
	}
	return newExecuteHandlers(c, hist), hist.Path()
}

func postJSON(t *testing.T, h http.HandlerFunc, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func TestExecuteSuccess(t *testing.T) {
	h, histPath := newTestExecuteHandlers(t)
	h.executeSQL = func(_ context.Context, _ aws.Config, target runner.Target, sql string) (*runner.Result, time.Duration, error) {
		if sql != "SELECT 1" {
			t.Errorf("unexpected sql: %q", sql)
		}
		if target.Database != "app" {
			t.Errorf("unexpected database: %q", target.Database)
		}
		return &runner.Result{
			Columns: []string{"n"},
			Rows:    [][]any{{int64(1)}},
			Updated: -1,
		}, 42 * time.Millisecond, nil
	}

	rr := postJSON(t, h.execute, "/api/execute", ExecuteRequest{
		Profile: "dev", Cluster: "arn:c", Secret: "arn:s", Database: "app",
		SQL: "SELECT 1",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp ExecuteResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Columns) != 1 || resp.Columns[0] != "n" {
		t.Errorf("columns = %v", resp.Columns)
	}
	if resp.DurationMS != 42 {
		t.Errorf("duration = %d, want 42", resp.DurationMS)
	}
	// history.jsonl should have one success entry.
	assertHistoryLines(t, histPath, 1, func(entries []history.Entry) {
		if !entries[0].Ok {
			t.Errorf("expected ok=true, got %+v", entries[0])
		}
	})
}

func TestExecuteEmptySQLRejected(t *testing.T) {
	h, histPath := newTestExecuteHandlers(t)
	h.executeSQL = func(_ context.Context, _ aws.Config, _ runner.Target, _ string) (*runner.Result, time.Duration, error) {
		// The handler may or may not call through — either way, the
		// response must be 400 and an empty SQL must not hit AWS.
		return nil, 0, runner.ErrEmptySQL
	}
	rr := postJSON(t, h.execute, "/api/execute", ExecuteRequest{
		Profile: "dev", Cluster: "arn:c", Secret: "arn:s", Database: "app",
		SQL: "   ",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
	// Empty SQL still records history (matches TUI behaviour) with Ok=false.
	assertHistoryLines(t, histPath, 1, func(entries []history.Entry) {
		if entries[0].Ok {
			t.Errorf("expected ok=false for empty SQL")
		}
	})
}

func TestExecuteAWSErrorMapsTo502AndHistoryLogged(t *testing.T) {
	h, histPath := newTestExecuteHandlers(t)
	h.executeSQL = func(_ context.Context, _ aws.Config, _ runner.Target, _ string) (*runner.Result, time.Duration, error) {
		return nil, 0, errors.New("AccessDenied: user has no rds-data permissions")
	}
	rr := postJSON(t, h.execute, "/api/execute", ExecuteRequest{
		Profile: "dev", Cluster: "arn:c", Secret: "arn:s", Database: "app",
		SQL: "SELECT 1",
	})
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
	var body ErrorDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != errCodeAWSError {
		t.Errorf("code = %s, want %s", body.Error.Code, errCodeAWSError)
	}
	assertHistoryLines(t, histPath, 1, func(entries []history.Entry) {
		if entries[0].Ok {
			t.Errorf("expected ok=false for AWS error")
		}
		if entries[0].ErrorMsg == "" {
			t.Errorf("expected error_msg populated")
		}
	})
}

func TestExecuteTimeoutMapsTo504(t *testing.T) {
	h, _ := newTestExecuteHandlers(t)
	h.executeSQL = func(_ context.Context, _ aws.Config, _ runner.Target, _ string) (*runner.Result, time.Duration, error) {
		return nil, 0, context.DeadlineExceeded
	}
	rr := postJSON(t, h.execute, "/api/execute", ExecuteRequest{
		Profile: "dev", Cluster: "arn:c", Secret: "arn:s", Database: "app",
		SQL: "SELECT pg_sleep(999)",
	})
	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", rr.Code)
	}
}

func TestExecuteMissingFieldsRejected(t *testing.T) {
	h, _ := newTestExecuteHandlers(t)

	cases := []ExecuteRequest{
		{Profile: "dev", Cluster: "arn:c", Secret: "arn:s", Database: "", SQL: "SELECT 1"},
		{Profile: "", Cluster: "arn:c", Secret: "arn:s", Database: "app", SQL: "SELECT 1"},
	}
	for _, tc := range cases {
		rr := postJSON(t, h.execute, "/api/execute", tc)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d for %+v, want 400", rr.Code, tc)
		}
	}
}

func assertHistoryLines(t *testing.T, path string, want int, check func([]history.Entry)) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open history: %v", err)
	}
	defer f.Close()
	var entries []history.Entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e history.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("history unmarshal: %v", err)
		}
		entries = append(entries, e)
	}
	if len(entries) != want {
		t.Fatalf("history lines = %d, want %d", len(entries), want)
	}
	if check != nil {
		check(entries)
	}
}
