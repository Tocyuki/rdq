package command

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Tocyuki/rdq/internal/history"
	"github.com/Tocyuki/rdq/internal/runner"
	"github.com/Tocyuki/rdq/internal/schema"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// fakeAsk returns a seam that emits a canned SQL string or error without
// touching the Bedrock SDK.
func fakeAsk(sql string, err error) func(ctx context.Context, cfg aws.Config, modelID, systemPrompt, userPrompt string) (string, error) {
	return func(ctx context.Context, cfg aws.Config, modelID, systemPrompt, userPrompt string) (string, error) {
		return sql, err
	}
}

// nilSchema returns a seam that always reports a cache miss without
// touching ~/.rdq/schema. The production code treats nil as "prompt
// without schema context" which is exactly what most ask tests want.
func nilSchema() func(cluster, database string) (*schema.Snapshot, error) {
	return func(cluster, database string) (*schema.Snapshot, error) {
		return nil, nil
	}
}

// fakeAictx returns a loader seam that always serves the same content.
// Used to verify the ask CLI threads aictx into the system prompt.
func fakeAictx(content string) func(cluster, database string) (string, error) {
	return func(cluster, database string) (string, error) {
		return content, nil
	}
}

// askGlobals mirrors defaultGlobals but also sets the Bedrock fields so
// the prompt/model validation passes. Tests that explicitly want to
// exercise the "no model" branch clear BedrockModel after calling this.
func askGlobals() *Globals {
	g := defaultGlobals()
	g.BedrockModel = "anthropic.claude-3-haiku-20240307-v1:0"
	g.BedrockLanguage = "English"
	return g
}

func TestRunAsk_SelectTable(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{int64(1), "alice"}},
		Updated: -1,
	}
	c := &AskCmd{
		Prompt:     []string{"list", "one", "user"},
		Output:     "table",
		askBedrock: fakeAsk("SELECT id, name FROM users LIMIT 1;", nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(res, nil, 10*time.Millisecond),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitSuccess, stderr.String())
	}
	if !strings.Contains(stderr.String(), "-- Generated SQL:") {
		t.Errorf("stderr missing generated SQL header:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "SELECT id, name FROM users LIMIT 1;") {
		t.Errorf("stderr missing generated SQL body:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "| alice") {
		t.Errorf("stdout missing table content:\n%s", stdout.String())
	}
}

func TestRunAsk_SelectJSON(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{
		Columns: []string{"n"},
		Rows:    [][]any{{int64(42)}},
		Updated: -1,
	}
	c := &AskCmd{
		Prompt:     []string{"give", "me", "42"},
		Output:     "json",
		askBedrock: fakeAsk("SELECT 42 AS n;", nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"n": 42`) {
		t.Errorf("expected JSON output, got:\n%s", stdout.String())
	}
}

func TestRunAsk_SelectCSV(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{int64(1), "alice"}, {int64(2), "bob"}},
		Updated: -1,
	}
	c := &AskCmd{
		Prompt:     []string{"users"},
		Output:     "csv",
		askBedrock: fakeAsk("SELECT id, name FROM users;", nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	want := "id,name\n1,alice\n2,bob\n"
	if stdout.String() != want {
		t.Errorf("stdout=%q, want %q", stdout.String(), want)
	}
}

func TestRunAsk_EmptyPrompt(t *testing.T) {
	setupHistoryEnv(t)
	c := &AskCmd{
		Prompt:     []string{"   ", "\t"},
		Output:     "table",
		askBedrock: fakeAsk("should not be called", errors.New("should not be called")),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(nil, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "prompt") {
		t.Errorf("expected prompt hint in stderr, got:\n%s", stderr.String())
	}
}

func TestRunAsk_NoBedrockModel(t *testing.T) {
	setupHistoryEnv(t)
	c := &AskCmd{
		Prompt:     []string{"anything"},
		Output:     "table",
		askBedrock: fakeAsk("should not be called", errors.New("should not be called")),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(nil, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	globals := askGlobals()
	globals.BedrockModel = ""
	var stdout, stderr bytes.Buffer
	code := runAsk(c, globals, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Bedrock model") {
		t.Errorf("expected Bedrock-model hint in stderr, got:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "rdq tui") {
		t.Errorf("expected suggestion to run rdq tui in stderr, got:\n%s", stderr.String())
	}
}

func TestRunAsk_AllCommentReply(t *testing.T) {
	histPath := setupHistoryEnv(t)
	cannotGenerate := "-- Cannot generate SQL.\n-- Reason: No matching table in the schema.\n-- Need: A table named `users` or an equivalent."
	c := &AskCmd{
		Prompt:     []string{"list", "tokyo", "users"},
		Output:     "table",
		askBedrock: fakeAsk(cannotGenerate, nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(nil, errors.New("should not be called"), 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "could not generate") {
		t.Errorf("expected user-facing error in stderr, got:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Reason:") {
		t.Errorf("expected the model's comment block to be preserved in stderr, got:\n%s", stderr.String())
	}
	// Nothing should be written to stdout and history must be untouched
	// because no SQL was executed.
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got:\n%s", stdout.String())
	}
	if data, _ := os.ReadFile(histPath); len(data) != 0 {
		t.Errorf("history should be empty when Bedrock cannot generate SQL, got:\n%s", data)
	}
}

func TestRunAsk_DryRunSkipsExecute(t *testing.T) {
	histPath := setupHistoryEnv(t)
	c := &AskCmd{
		Prompt:     []string{"count", "users"},
		Output:     "table",
		DryRun:     true,
		askBedrock: fakeAsk("SELECT COUNT(*) FROM users;", nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(nil, errors.New("should not be called"), 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	// The SQL must land on stdout so `rdq ask -n ... | rdq exec --file -`
	// receives a clean SQL stream. Adding the `-- Generated SQL:` header
	// would contaminate that pipe.
	if !strings.Contains(stdout.String(), "SELECT COUNT(*) FROM users;") {
		t.Errorf("stdout missing generated SQL for pipe workflow, got:\n%s", stdout.String())
	}
	if strings.Contains(stderr.String(), "-- Generated SQL:") {
		t.Errorf("stderr should not include header in --dry-run (keeps pipe clean), got:\n%s", stderr.String())
	}
	if data, _ := os.ReadFile(histPath); len(data) != 0 {
		t.Errorf("history should be empty in --dry-run, got:\n%s", data)
	}
}

func TestRunAsk_ReadOnlyBlocksWrite(t *testing.T) {
	histPath := setupHistoryEnv(t)
	c := &AskCmd{
		Prompt:     []string{"delete", "sessions"},
		Output:     "table",
		askBedrock: fakeAsk("DELETE FROM sessions WHERE id = 1;", nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(nil, errors.New("should not be called"), 0),
		loadState:  readOnlyState(nil), // RO default
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitReadOnly {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitReadOnly, stderr.String())
	}
	if !strings.Contains(stderr.String(), "read-only") {
		t.Errorf("expected read-only message, got:\n%s", stderr.String())
	}
	// Blocked attempts must be recorded in history so users see them in
	// the picker the same way exec does.
	data, _ := os.ReadFile(histPath)
	if !strings.Contains(string(data), "read-only") {
		t.Errorf("expected read-only block to be recorded in history, got:\n%s", data)
	}
}

func TestRunAsk_DestructiveNonTTYNeedsYes(t *testing.T) {
	histPath := setupHistoryEnv(t)
	allow := false
	c := &AskCmd{
		Prompt:     []string{"delete", "everything"},
		Output:     "table",
		askBedrock: fakeAsk("DELETE FROM users;", nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(nil, errors.New("should not be called"), 0),
		loadState:  readOnlyState(&allow),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitNotConfirmed {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitNotConfirmed, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--yes") {
		t.Errorf("expected --yes hint in stderr, got:\n%s", stderr.String())
	}
	if data, _ := os.ReadFile(histPath); len(data) != 0 {
		t.Errorf("unconfirmed attempts must not touch history, got:\n%s", data)
	}
}

func TestRunAsk_DestructiveInteractiveYes(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{Updated: 4}
	allow := false
	c := &AskCmd{
		Prompt:     []string{"truncate", "logs"},
		Output:     "table",
		askBedrock: fakeAsk("TRUNCATE logs;", nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(&allow),
		isTerminal: func(io.Reader) bool { return true },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader("y\n"), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "(4 rows affected)") {
		t.Errorf("expected affected count on stderr, got:\n%s", stderr.String())
	}
}

func TestRunAsk_QuietSuppressesHeader(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{
		Columns: []string{"n"},
		Rows:    [][]any{{int64(1)}},
		Updated: -1,
	}
	c := &AskCmd{
		Prompt:     []string{"one"},
		Output:     "json",
		Quiet:      true,
		askBedrock: fakeAsk("SELECT 1 AS n;", nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "-- Generated SQL:") {
		t.Errorf("--quiet should suppress the generated-SQL echo, got:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "SELECT 1 AS n;") {
		t.Errorf("--quiet should suppress the SQL body too, got:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"n": 1`) {
		t.Errorf("stdout should still carry the result, got:\n%s", stdout.String())
	}
}

func TestRunAsk_QuietStillShowsErrors(t *testing.T) {
	setupHistoryEnv(t)
	c := &AskCmd{
		Prompt:     []string{"fail"},
		Output:     "table",
		Quiet:      true,
		askBedrock: fakeAsk("", errors.New("bedrock converse: AccessDeniedException")),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(nil, errors.New("should not be called"), 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "AccessDeniedException") {
		t.Errorf("--quiet must NOT suppress error messages, got:\n%s", stderr.String())
	}
}

func TestRunAsk_BedrockTimeout(t *testing.T) {
	setupHistoryEnv(t)
	c := &AskCmd{
		Prompt:     []string{"slow"},
		Output:     "table",
		askBedrock: fakeAsk("", context.DeadlineExceeded),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(nil, errors.New("should not be called"), 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitTimeout {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitTimeout, stderr.String())
	}
}

func TestRunAsk_BedrockError(t *testing.T) {
	setupHistoryEnv(t)
	c := &AskCmd{
		Prompt:     []string{"fail"},
		Output:     "table",
		askBedrock: fakeAsk("", errors.New("bedrock converse: AccessDeniedException")),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(nil, errors.New("should not be called"), 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "AccessDeniedException") {
		t.Errorf("expected underlying error surface, got:\n%s", stderr.String())
	}
}

func TestRunAsk_ExecuteTimeout(t *testing.T) {
	setupHistoryEnv(t)
	c := &AskCmd{
		Prompt:     []string{"slow"},
		Output:     "table",
		askBedrock: fakeAsk("SELECT pg_sleep(99999);", nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(nil, context.DeadlineExceeded, 2*time.Minute),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitTimeout {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitTimeout, stderr.String())
	}
}

func TestRunAsk_EphemeralSkipsHistory(t *testing.T) {
	histPath := setupHistoryEnv(t)
	res := &runner.Result{Columns: []string{"n"}, Rows: [][]any{{int64(1)}}, Updated: -1}
	c := &AskCmd{
		Prompt:     []string{"count", "one"},
		Output:     "json",
		askBedrock: fakeAsk("SELECT 1 AS n;", nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	globals := askGlobals()
	globals.Profile = "" // ephemeral
	var stdout, stderr bytes.Buffer
	code := runAsk(c, globals, strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if data, _ := os.ReadFile(histPath); len(data) != 0 {
		t.Errorf("history file should stay empty in ephemeral mode, got:\n%s", data)
	}
}

func TestRunAsk_HistoryRecordedOnSuccess(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{Columns: []string{"n"}, Rows: [][]any{{int64(1)}}, Updated: -1}
	c := &AskCmd{
		Prompt:     []string{"select", "one"},
		Output:     "json",
		askBedrock: fakeAsk("SELECT 1 AS n;", nil),
		loadSchema: nilSchema(),
		executeSQL: fakeExecute(res, nil, 12*time.Millisecond),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	store, err := history.New()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := store.Load("test-profile", "rdq_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !entries[0].Ok || entries[0].SQL != "SELECT 1 AS n;" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}
}

func TestIsAllSQLComments(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"all comments", "-- a\n-- b\n-- c", true},
		{"comments with blank lines", "-- a\n\n-- b\n\n", true},
		{"single comment", "-- nope", true},
		{"leading whitespace before comment", "  -- indented", true},
		{"mixed sql and comments", "-- header\nSELECT 1;", false},
		{"trailing comment after sql", "SELECT 1;\n-- done", false},
		{"real sql", "SELECT id FROM users;", false},
		{"empty", "", false},
		{"only whitespace", "   \n\t\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAllSQLComments(tc.in); got != tc.want {
				t.Errorf("isAllSQLComments(%q)=%v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRunAsk_PassesUserContextToBedrock(t *testing.T) {
	setupHistoryEnv(t)
	res := &runner.Result{Columns: []string{"n"}, Rows: [][]any{{int64(1)}}, Updated: -1}

	var capturedSystemPrompt string
	captureAsk := func(ctx context.Context, cfg aws.Config, modelID, systemPrompt, userPrompt string) (string, error) {
		capturedSystemPrompt = systemPrompt
		return "SELECT 1 AS n;", nil
	}

	c := &AskCmd{
		Prompt:     []string{"give", "me", "active", "users"},
		Output:     "json",
		askBedrock: captureAsk,
		loadSchema: nilSchema(),
		loadAictx:  fakeAictx("active user = last_login_at within 30 days"),
		executeSQL: fakeExecute(res, nil, 0),
		loadState:  readOnlyState(nil),
		isTerminal: func(io.Reader) bool { return false },
	}
	var stdout, stderr bytes.Buffer
	code := runAsk(c, askGlobals(), strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(capturedSystemPrompt, "User-provided context:") {
		t.Errorf("system prompt missing user-context heading:\n%s", capturedSystemPrompt)
	}
	if !strings.Contains(capturedSystemPrompt, "active user = last_login_at within 30 days") {
		t.Errorf("system prompt missing user-context body:\n%s", capturedSystemPrompt)
	}
}
