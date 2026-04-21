package command

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tocyuki/rdq/internal/history"
	"github.com/Tocyuki/rdq/internal/runner"
	"github.com/Tocyuki/rdq/internal/state"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// setupHistoryEnv points RDQ_HISTORY_FILE at a throw-away file and returns
// the path so the test can assert on its contents.
func setupHistoryEnv(t *testing.T) string {
	t.Helper()
	histPath := filepath.Join(t.TempDir(), "history.jsonl")
	t.Setenv("RDQ_HISTORY_FILE", histPath)
	return histPath
}

// defaultGlobals returns Globals populated with stable non-empty values so
// connection fields are not confused with ephemeral mode (Profile == "").
func defaultGlobals() *Globals {
	return &Globals{
		Profile:    "test-profile",
		AWSConfig:  aws.Config{Region: "ap-northeast-1"},
		ClusterArn: "arn:aws:rds:ap-northeast-1:123:cluster:test",
		SecretArn:  "arn:aws:secretsmanager:ap-northeast-1:123:secret:test",
		Database:   "rdq_test",
	}
}

// readOnlyState returns a loadState seam that reports the given policy.
// Passing nil produces a profile whose IsReadOnly is unset, which exercises
// the "default to ON" branch.
func readOnlyState(policy *bool) func() (*state.State, error) {
	return func() (*state.State, error) {
		st := &state.State{Profiles: map[string]state.ProfileState{}}
		st.Set("test-profile", state.ProfileState{IsReadOnly: policy})
		return st, nil
	}
}

// fakeExecute returns a seam that emits a canned Result or error.
func fakeExecute(res *runner.Result, err error, elapsed time.Duration) func(ctx context.Context, cfg aws.Config, target runner.Target, sql string) (*runner.Result, time.Duration, error) {
	return func(ctx context.Context, cfg aws.Config, target runner.Target, sql string) (*runner.Result, time.Duration, error) {
		return res, elapsed, err
	}
}

func TestRunExec_SelectTable(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{int64(1), "alice"}},
		Updated: -1,
	}
	c := &ExecCmd{
		SQL:        "SELECT id, name FROM users",
		Output:     "table",
		executeSQL: fakeExecute(res, nil, 10*time.Millisecond),
		loadState:  readOnlyState(nil), // default (RO)
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitSuccess, stderr.String())
	}
	if !strings.Contains(stdout.String(), "| alice") {
		t.Errorf("stdout missing table content:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "(1 row)") {
		t.Errorf("stdout missing footer:\n%s", stdout.String())
	}
}

func TestRunExec_SelectJSON(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{
		Columns: []string{"id"},
		Rows:    [][]any{{int64(42)}},
		Updated: -1,
	}
	c := &ExecCmd{
		SQL:        "SELECT 42",
		Output:     "json",
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id": 42`) {
		t.Errorf("expected JSON with id:42, got:\n%s", stdout.String())
	}
}

func TestRunExec_SelectCSV(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{int64(1), "alice"}, {int64(2), "bob"}},
		Updated: -1,
	}
	c := &ExecCmd{
		SQL:        "SELECT id, name FROM users",
		Output:     "csv",
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	want := "id,name\n1,alice\n2,bob\n"
	if stdout.String() != want {
		t.Errorf("stdout=%q, want %q", stdout.String(), want)
	}
}

func TestRunExec_UpdateWritesAffectedToStderr(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{
		Columns: nil,
		Rows:    nil,
		Updated: 3,
	}
	allow := false
	c := &ExecCmd{
		SQL:        "UPDATE users SET active = true WHERE id < 5",
		Output:     "table",
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(&allow),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout for write, got:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "(3 rows affected)") {
		t.Errorf("expected affected count on stderr, got:\n%s", stderr.String())
	}
}

func TestRunExec_ReadOnlyBlocksWrite(t *testing.T) {
	histPath := setupHistoryEnv(t)
	c := &ExecCmd{
		SQL:        "DELETE FROM users WHERE id = 1",
		Output:     "table",
		executeSQL: fakeExecute(nil, errors.New("should not be called"), 0),
		loadState:  readOnlyState(nil), // RO by default
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitReadOnly {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitReadOnly, stderr.String())
	}
	if !strings.Contains(stderr.String(), "read-only") {
		t.Errorf("expected read-only message, got:\n%s", stderr.String())
	}
	// History should record the blocked attempt so users see it in the
	// picker the same way the GUI does.
	data, _ := os.ReadFile(histPath)
	if !strings.Contains(string(data), "read-only") {
		t.Errorf("expected read-only block to be recorded in history, got:\n%s", data)
	}
}

func TestRunExec_ReadOnlyAllowsWriteWhenDisabled(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{Updated: 1}
	allow := false
	c := &ExecCmd{
		SQL:        "INSERT INTO users (id) VALUES (1)",
		Output:     "table",
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(&allow),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
}

func TestRunExec_DestructiveNonTTY(t *testing.T) {
	histPath := setupHistoryEnv(t)
	allow := false
	c := &ExecCmd{
		SQL:        "DELETE FROM users",
		Output:     "table",
		executeSQL: fakeExecute(nil, errors.New("should not be called"), 0),
		loadState:  readOnlyState(&allow),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitNotConfirmed {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitNotConfirmed, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--yes") {
		t.Errorf("expected --yes hint in stderr, got:\n%s", stderr.String())
	}
	// Unconfirmed attempts must NOT be recorded (matches handlers_execute.go).
	data, _ := os.ReadFile(histPath)
	if len(data) != 0 {
		t.Errorf("history should be empty, got:\n%s", data)
	}
}

func TestRunExec_DestructiveWithYes(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{Updated: 7}
	allow := false
	c := &ExecCmd{
		SQL:        "DELETE FROM users",
		Output:     "table",
		Yes:        true,
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(&allow),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "(7 rows affected)") {
		t.Errorf("expected affected count, got:\n%s", stderr.String())
	}
}

func TestRunExec_DestructiveInteractiveYes(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{Updated: 2}
	allow := false
	c := &ExecCmd{
		SQL:        "TRUNCATE logs",
		Output:     "table",
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(&allow),
		isTerminal: func(io.Reader) bool { return true },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader("y\n"), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
}

func TestRunExec_DestructiveInteractiveNo(t *testing.T) {
	setupHistoryEnv(t)
	allow := false
	c := &ExecCmd{
		SQL:        "TRUNCATE logs",
		Output:     "table",
		executeSQL: fakeExecute(nil, errors.New("should not be called"), 0),
		loadState:  readOnlyState(&allow),
		isTerminal: func(io.Reader) bool { return true },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader("n\n"), &stdout, &stderr)
	if code != exitNotConfirmed {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitNotConfirmed, stderr.String())
	}
}

func TestRunExec_TimeoutExit5(t *testing.T) {
	setupHistoryEnv(t)
	allow := false
	c := &ExecCmd{
		SQL:        "SELECT pg_sleep(99999)",
		Output:     "table",
		executeSQL: fakeExecute(nil, context.DeadlineExceeded, 2*time.Minute),
		loadState:  readOnlyState(&allow),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitTimeout {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitTimeout, stderr.String())
	}
}

func TestRunExec_EmptySQL(t *testing.T) {
	setupHistoryEnv(t)
	c := &ExecCmd{
		Output:     "table",
		executeSQL: fakeExecute(nil, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitUsage, stderr.String())
	}
}

func TestRunExec_MutualExclusion(t *testing.T) {
	setupHistoryEnv(t)
	c := &ExecCmd{
		SQL:        "SELECT 1",
		File:       "/tmp/nonexistent.sql",
		Output:     "table",
		executeSQL: fakeExecute(nil, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Errorf("expected mutual exclusion message, got:\n%s", stderr.String())
	}
}

func TestRunExec_FileRead(t *testing.T) {
	setupHistoryEnv(t)
	dir := t.TempDir()
	sqlPath := filepath.Join(dir, "query.sql")
	if err := os.WriteFile(sqlPath, []byte("SELECT 1 AS n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := &runner.Result{
		Columns: []string{"n"},
		Rows:    [][]any{{int64(1)}},
		Updated: -1,
	}
	c := &ExecCmd{
		File:       sqlPath,
		Output:     "json",
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"n": 1`) {
		t.Errorf("unexpected stdout:\n%s", stdout.String())
	}
}

func TestRunExec_Stdin(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{
		Columns: []string{"n"},
		Rows:    [][]any{{int64(1)}},
		Updated: -1,
	}
	c := &ExecCmd{
		File:       "-",
		Output:     "json",
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader("SELECT 1 AS n"), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"n": 1`) {
		t.Errorf("unexpected stdout:\n%s", stdout.String())
	}
}

func TestRunExec_EphemeralSkipsHistory(t *testing.T) {
	histPath := setupHistoryEnv(t)
	res := &runner.Result{Columns: []string{"n"}, Rows: [][]any{{int64(1)}}, Updated: -1}
	c := &ExecCmd{
		SQL:        "SELECT 1",
		Output:     "json",
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	globals := defaultGlobals()
	globals.Profile = "" // ephemeral
	var stdout, stderr bytes.Buffer
	code := runExec(c, globals, strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	data, _ := os.ReadFile(histPath)
	if len(data) != 0 {
		t.Errorf("history file should stay empty in ephemeral mode, got:\n%s", data)
	}
}

func TestRunExec_HistoryRecordedOnSuccess(t *testing.T) {
	histPath := setupHistoryEnv(t)
	res := &runner.Result{Columns: []string{"n"}, Rows: [][]any{{int64(1)}}, Updated: -1}
	c := &ExecCmd{
		SQL:        "SELECT 1",
		Output:     "json",
		executeSQL: fakeExecute(res, nil, 15*time.Millisecond),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runExec(c, defaultGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	// Cross-check by reading back via history.Store so format assumptions
	// stay in one place.
	store, err := history.New()
	if err != nil {
		t.Fatal(err)
	}
	if store.Path() != histPath {
		t.Errorf("history path mismatch: got %s, want %s", store.Path(), histPath)
	}
	entries, err := store.Load("test-profile", "rdq_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !entries[0].Ok || entries[0].SQL != "SELECT 1" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}
}

func TestDefaultIsTerminal_NonFile(t *testing.T) {
	if defaultIsTerminal(strings.NewReader("")) {
		t.Error("strings.Reader should not be reported as a terminal")
	}
	if defaultIsTerminal(&bytes.Buffer{}) {
		t.Error("bytes.Buffer should not be reported as a terminal")
	}
}
