package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Tocyuki/rdq/internal/bedrock"
	"github.com/Tocyuki/rdq/internal/runner"
	"github.com/Tocyuki/rdq/internal/schema"
	"github.com/Tocyuki/rdq/internal/state"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// bedrockTimeout caps how long a single Converse call is allowed to run.
// Mirrors internal/tui/ai.go askCmd so TUI and CLI agree on the wait.
const bedrockTimeout = 90 * time.Second

// AskCmd is the Kong struct for `rdq ask`. It turns a natural-language
// prompt into SQL via Amazon Bedrock and then funnels the generated
// statement through the same downstream pipeline exec uses (read-only
// gate → destructive confirmation → Data API → history → render), so a
// user who flips read-only on in the TUI sees identical behaviour here.
//
// The unexported seams exist so ask_test.go can exercise every branch
// without hitting AWS. Production callers should never set them.
type AskCmd struct {
	Prompt []string `arg:"" help:"Natural-language prompt. Multiple tokens are joined with spaces so quoting is optional."`
	Output string   `short:"o" default:"table" enum:"table,json,csv" help:"Output format: table, json, or csv."`
	DryRun bool     `short:"n" help:"Print the generated SQL to stdout and exit without executing. Handy for piping into 'rdq exec --file -'."`
	Quiet  bool     `short:"q" help:"Suppress the '-- Generated SQL:' echo on stderr. Error messages and '(N rows affected)' still go to stderr."`
	Yes    bool     `short:"y" help:"Skip confirmation for destructive statements (DELETE/UPDATE without WHERE, TRUNCATE). Required in non-interactive shells for those statements."`

	askBedrock func(ctx context.Context, cfg aws.Config, modelID, systemPrompt, userPrompt string) (string, error)
	loadSchema func(cluster, database string) (*schema.Snapshot, error)
	executeSQL func(ctx context.Context, cfg aws.Config, target runner.Target, sql string) (*runner.Result, time.Duration, error)
	loadState  func() (*state.State, error)
	isTerminal func(io.Reader) bool
}

// Run is the Kong entrypoint. It delegates to runAsk for all real logic so
// tests can drive the pure function directly and observe exit codes without
// terminating the test binary via os.Exit.
func (c *AskCmd) Run(globals *Globals) error {
	os.Exit(runAsk(c, globals, os.Stdin, os.Stdout, os.Stderr))
	return nil
}

// runAsk is the testable core of `rdq ask`. The pipeline:
//
//  1. Validate prompt and Bedrock model.
//  2. Load the cached schema snapshot (if present); cache-miss is OK.
//  3. Build the SQL-assistant system prompt and Converse with Bedrock.
//  4. Detect the "all comments" fallback the system prompt uses when the
//     model cannot generate runnable SQL — executing that would look like
//     a silent success. Surface it as a user error instead.
//  5. Echo the generated SQL to stderr (so stdout stays a clean data stream).
//  6. Dry-run stops here; otherwise hand off to the same read-only +
//     destructive-confirmation + execute + history + render pipeline
//     exec uses.
func runAsk(c *AskCmd, globals *Globals, stdin io.Reader, stdout, stderr io.Writer) int {
	c.setDefaultSeams()

	prompt := strings.TrimSpace(strings.Join(c.Prompt, " "))
	if prompt == "" {
		fmt.Fprintln(stderr, "rdq ask: provide a natural-language prompt as the positional argument.")
		return exitUsage
	}
	if strings.TrimSpace(globals.BedrockModel) == "" {
		fmt.Fprintln(stderr, "rdq ask: no Bedrock model configured.")
		fmt.Fprintln(stderr, "rdq ask: run `rdq tui` once to pick a model (it is cached per profile), or pass --bedrock-model <id>.")
		return exitUsage
	}

	// Cached snapshot only — ask CLI never blocks on a live
	// information_schema fetch. The prompt template already handles a
	// nil snapshot by omitting the schema block, and on a miss the
	// model will most likely hit the "Cannot generate SQL" path which
	// we surface as an error below.
	var snapshot *schema.Snapshot
	if globals.ClusterArn != "" && globals.Database != "" {
		if snap, err := c.loadSchema(globals.ClusterArn, globals.Database); err == nil {
			snapshot = snap
		}
	}

	systemPrompt := bedrock.BuildSystemPrompt(globals.Database, globals.BedrockLanguage, snapshot)

	bedrockCtx, cancelBedrock := context.WithTimeout(context.Background(), bedrockTimeout)
	defer cancelBedrock()
	sql, err := c.askBedrock(bedrockCtx, globals.AWSConfig, globals.BedrockModel, systemPrompt, prompt)
	if err != nil {
		fmt.Fprintln(stderr, "rdq ask: "+err.Error())
		if errors.Is(err, context.DeadlineExceeded) {
			return exitTimeout
		}
		return exitError
	}

	sql = strings.TrimSpace(sql)
	if sql == "" || isAllSQLComments(sql) {
		fmt.Fprintln(stderr, "rdq ask: the model could not generate runnable SQL.")
		if sql != "" {
			fmt.Fprintln(stderr, sql)
		}
		fmt.Fprintln(stderr, "rdq ask: refine the prompt, or run `rdq tui` once to seed the schema cache for this database.")
		return exitError
	}

	// Output targets differ between modes so `rdq ask -n ... | rdq exec
	// --file -` is a usable pipe:
	//   - dry-run: SQL goes to stdout, bare (no header), so stdin on the
	//     right side of the pipe receives a clean SQL stream. --quiet is
	//     ignored here because the whole point of dry-run is to emit SQL.
	//   - execute: SQL is echoed to stderr as `-- Generated SQL:\n<sql>`
	//     so stdout stays a clean result-data stream, matching exec's
	//     "rows affected on stderr" contract. --quiet skips this echo
	//     without affecting error messages.
	if c.DryRun {
		fmt.Fprintln(stdout, sql)
		return exitSuccess
	}
	if !c.Quiet {
		fmt.Fprintln(stderr, "-- Generated SQL:")
		fmt.Fprintln(stderr, sql)
	}

	if isReadOnlyProfile(c.loadState, globals.Profile) && !runner.IsReadOnlySQL(sql) {
		fmt.Fprintln(stderr, "rdq ask: "+runner.ErrWriteBlocked.Error())
		fmt.Fprintln(stderr, "rdq ask: disable read-only mode in the TUI (F8) or GUI settings, then retry.")
		recordHistory(globals, sql, runner.ErrWriteBlocked, 0)
		return exitReadOnly
	}

	if need, reason := runner.NeedsConfirmation(sql); need && !c.Yes {
		if !c.isTerminal(stdin) {
			fmt.Fprintln(stderr, "rdq ask: destructive statement requires --yes in non-interactive mode.")
			fmt.Fprintln(stderr, "rdq ask: "+reason)
			return exitNotConfirmed
		}
		if !promptConfirm(stdin, stderr, reason) {
			fmt.Fprintln(stderr, "rdq ask: aborted by user.")
			return exitNotConfirmed
		}
	}

	execCtx, cancelExec := context.WithTimeout(context.Background(), runner.ExecuteTimeout)
	defer cancelExec()

	target := runner.Target{
		Profile:  globals.Profile,
		Region:   globals.AWSConfig.Region,
		Cluster:  globals.ClusterArn,
		Secret:   globals.SecretArn,
		Database: globals.Database,
	}
	res, elapsed, execErr := c.executeSQL(execCtx, globals.AWSConfig, target, sql)
	recordHistory(globals, sql, execErr, elapsed)

	if execErr != nil {
		fmt.Fprintln(stderr, "rdq ask: "+execErr.Error())
		switch {
		case errors.Is(execErr, context.DeadlineExceeded):
			return exitTimeout
		case errors.Is(execErr, runner.ErrEmptySQL):
			return exitUsage
		default:
			return exitError
		}
	}

	if err := renderResult(stdout, stderr, res, c.Output); err != nil {
		fmt.Fprintln(stderr, "rdq ask: "+err.Error())
		return exitError
	}
	return exitSuccess
}

// setDefaultSeams wires production implementations for any seam the caller
// left at its zero value. Tests inject fakes before calling runAsk.
func (c *AskCmd) setDefaultSeams() {
	if c.askBedrock == nil {
		c.askBedrock = defaultAskBedrock
	}
	if c.loadSchema == nil {
		c.loadSchema = schema.LoadCache
	}
	if c.executeSQL == nil {
		c.executeSQL = defaultExecuteSQL
	}
	if c.loadState == nil {
		c.loadState = state.Load
	}
	if c.isTerminal == nil {
		c.isTerminal = defaultIsTerminal
	}
}

// defaultAskBedrock wraps bedrock.Client.Ask so the seam signature matches
// the per-invocation call. Mirrors defaultExecuteSQL's thin-adapter role:
// a fresh Bedrock client per run keeps `command/` independent of any
// long-lived SDK client ownership decisions the TUI / GUI might make.
func defaultAskBedrock(ctx context.Context, cfg aws.Config, modelID, systemPrompt, userPrompt string) (string, error) {
	client := bedrock.New(cfg)
	messages := []bedrock.Message{{Role: bedrock.RoleUser, Text: userPrompt}}
	return client.Ask(ctx, modelID, systemPrompt, messages)
}

// isAllSQLComments reports whether every non-empty, non-whitespace line of
// s starts with "--". When Bedrock cannot resolve the request to runnable
// SQL the system prompt (internal/bedrock/prompt.go) instructs it to emit
// a block where every line is an SQL comment. Without this check the
// Data API would accept the payload, return zero rows, and make the run
// look like a silent success.
func isAllSQLComments(s string) bool {
	lines := strings.Split(s, "\n")
	sawContent := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		sawContent = true
		if !strings.HasPrefix(trimmed, "--") {
			return false
		}
	}
	return sawContent
}
