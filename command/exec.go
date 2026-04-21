package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Tocyuki/rdq/internal/history"
	"github.com/Tocyuki/rdq/internal/runner"
	"github.com/Tocyuki/rdq/internal/state"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	"golang.org/x/term"
)

// Exit codes for `rdq exec`. They are documented in README.md so users can
// wire them into CI pipelines; keep the two in sync when adjusting either.
const (
	exitSuccess        = 0
	exitError          = 1
	exitUsage          = 2
	exitReadOnly       = 3
	exitNotConfirmed   = 4
	exitTimeout        = 5
	stdinSentinel      = "-"
	defaultOutputTable = "table"
)

// ExecCmd is the Kong struct for `rdq exec`. The stub was replaced with a
// one-shot runner that reuses the same engine the TUI and GUI talk to.
//
// SQL is optional as a positional arg so users can supply it via --file
// instead. Mutually exclusive with --file; both empty or both set is a
// usage error.
//
// The unexported seams (executeSQL, loadState, isTerminal) are intentionally
// left unexported: they exist only so exec_test.go can exercise every branch
// without hitting AWS. Production callers should never set them.
type ExecCmd struct {
	SQL    string `arg:"" optional:"" help:"SQL statement to execute. Mutually exclusive with --file."`
	File   string `short:"f" help:"Read SQL from file path. Use - for stdin."`
	Output string `short:"o" default:"table" enum:"table,json,csv" help:"Output format: table, json, or csv."`
	Yes    bool   `short:"y" help:"Skip confirmation for destructive statements (DELETE/UPDATE without WHERE, TRUNCATE). Required in non-interactive shells for those statements."`

	executeSQL func(ctx context.Context, cfg aws.Config, target runner.Target, sql string) (*runner.Result, time.Duration, error)
	loadState  func() (*state.State, error)
	isTerminal func(io.Reader) bool
}

// Run is the Kong entrypoint. It delegates to runExec for all real logic so
// tests can drive the pure function directly and observe exit codes without
// terminating the test binary via os.Exit.
func (c *ExecCmd) Run(globals *Globals) error {
	os.Exit(runExec(c, globals, os.Stdin, os.Stdout, os.Stderr))
	return nil
}

// runExec is the testable core of `rdq exec`. It never calls os.Exit or
// touches os.Std* directly so test code can feed in its own buffers and
// assert on the exit code that the real Run() would have produced.
//
// The pipeline mirrors internal/server/handlers_execute.go so the three
// surfaces (TUI, GUI, CLI) agree on every policy: read-only gating first,
// destructive confirmation second, execute third, history fourth. Diverging
// here would surprise users who flip read-only in the GUI and expect the
// CLI to honour it.
func runExec(c *ExecCmd, globals *Globals, stdin io.Reader, stdout, stderr io.Writer) int {
	c.setDefaultSeams()

	sql, err := c.resolveSQLInput(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "rdq exec: "+err.Error())
		return exitUsage
	}
	if strings.TrimSpace(sql) == "" {
		fmt.Fprintln(stderr, "rdq exec: "+runner.ErrEmptySQL.Error())
		return exitUsage
	}

	readOnly := c.isReadOnlyProfile(globals.Profile)
	if readOnly && !runner.IsReadOnlySQL(sql) {
		fmt.Fprintln(stderr, "rdq exec: "+runner.ErrWriteBlocked.Error())
		fmt.Fprintln(stderr, "rdq exec: disable read-only mode in the TUI (F8) or GUI settings, then retry.")
		recordHistory(globals, sql, runner.ErrWriteBlocked, 0)
		return exitReadOnly
	}

	if need, reason := runner.NeedsConfirmation(sql); need && !c.Yes {
		if !c.isTerminal(stdin) {
			fmt.Fprintln(stderr, "rdq exec: destructive statement requires --yes in non-interactive mode.")
			fmt.Fprintln(stderr, "rdq exec: "+reason)
			return exitNotConfirmed
		}
		if !promptConfirm(stdin, stderr, reason) {
			fmt.Fprintln(stderr, "rdq exec: aborted by user.")
			return exitNotConfirmed
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), runner.ExecuteTimeout)
	defer cancel()

	target := runner.Target{
		Profile:  globals.Profile,
		Region:   globals.AWSConfig.Region,
		Cluster:  globals.ClusterArn,
		Secret:   globals.SecretArn,
		Database: globals.Database,
	}
	res, elapsed, execErr := c.executeSQL(ctx, globals.AWSConfig, target, sql)
	recordHistory(globals, sql, execErr, elapsed)

	if execErr != nil {
		fmt.Fprintln(stderr, "rdq exec: "+execErr.Error())
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
		fmt.Fprintln(stderr, "rdq exec: "+err.Error())
		return exitError
	}
	return exitSuccess
}

// setDefaultSeams lazily wires the production implementations for any seam
// the caller left at its zero value. Production callers never touch these;
// tests inject fakes before calling runExec.
func (c *ExecCmd) setDefaultSeams() {
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

// defaultExecuteSQL mirrors internal/server/handlers_execute.go's
// defaultExecuteSQL. Both construct a fresh rdsdata client from the shared
// aws.Config and delegate to the shared runner. Duplicating this thin
// adapter (rather than exporting a helper from internal/runner) keeps
// command/ free of server/ dependencies and vice versa.
func defaultExecuteSQL(ctx context.Context, cfg aws.Config, target runner.Target, sql string) (*runner.Result, time.Duration, error) {
	client := rdsdata.NewFromConfig(cfg)
	return runner.ExecuteSQL(ctx, client, target, sql)
}

// defaultIsTerminal reports whether r is the real terminal stdin.
// We only treat *os.File backed by fd 0 as a TTY; arbitrary io.Readers
// (bytes.Buffer in tests, pipes from shells) correctly return false.
func defaultIsTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// resolveSQLInput picks one of three sources: positional arg, --file <path>,
// or --file - (stdin). Two sources set simultaneously is a usage error so
// the user doesn't silently lose the one that gets ignored.
//
// The return value is the raw file/stdin content; TrimSpace happens later
// in runExec so we can distinguish "empty file" from "no SQL provided" here.
func (c *ExecCmd) resolveSQLInput(stdin io.Reader) (string, error) {
	hasArg := strings.TrimSpace(c.SQL) != ""
	hasFile := c.File != ""

	if hasArg && hasFile {
		return "", errors.New("positional SQL and --file are mutually exclusive")
	}
	if hasArg {
		return c.SQL, nil
	}
	if hasFile {
		if c.File == stdinSentinel {
			data, err := io.ReadAll(stdin)
			if err != nil {
				return "", fmt.Errorf("read stdin: %w", err)
			}
			return string(data), nil
		}
		data, err := os.ReadFile(c.File)
		if err != nil {
			return "", fmt.Errorf("read file %s: %w", c.File, err)
		}
		return string(data), nil
	}
	return "", errors.New("provide SQL as a positional argument, --file <path>, or --file -")
}

// isReadOnlyProfile resolves the per-profile read-only policy from
// state.json. A nil IsReadOnly (never toggled) defaults to TRUE to match
// the TUI/GUI policy — fresh installs are always safe until the user
// explicitly opts into writes.
func (c *ExecCmd) isReadOnlyProfile(profile string) bool {
	st, err := c.loadState()
	if err != nil {
		// Fail closed — losing state.json should not open destructive
		// writes that the user had gated behind it.
		log.Printf("rdq exec: state.Load failed, defaulting to read-only: %v", err)
		return true
	}
	ps := st.Get(profile)
	if ps.IsReadOnly == nil {
		return true
	}
	return *ps.IsReadOnly
}

// promptConfirm shows the destructive-statement reason on stderr and reads a
// single-line y/N answer from stdin. Only an exact "y" or "yes" (case
// insensitive) counts as consent; anything else — including EOF on stdin —
// cancels the run.
func promptConfirm(stdin io.Reader, stderr io.Writer, reason string) bool {
	fmt.Fprintln(stderr, "rdq exec: "+reason)
	fmt.Fprint(stderr, "rdq exec: proceed? [y/N] ")
	br := bufio.NewReader(stdin)
	line, err := br.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes"
}

// recordHistory appends a single execution entry. Ephemeral mode (empty
// profile) deliberately skips the append so direct-credentials runs leave
// no on-disk trace, matching the TUI/GUI policy. A broken history file
// should never fail the user's actual query result, so errors are logged
// rather than propagated.
func recordHistory(globals *Globals, sql string, execErr error, elapsed time.Duration) {
	if globals.Profile == "" {
		return
	}
	store, err := history.New()
	if err != nil {
		log.Printf("rdq exec: history.New failed: %v", err)
		return
	}
	entry := history.Entry{
		Profile:    globals.Profile,
		Database:   globals.Database,
		SQL:        sql,
		At:         time.Now(),
		Ok:         execErr == nil,
		DurationMS: elapsed.Milliseconds(),
	}
	if execErr != nil {
		entry.ErrorMsg = execErr.Error()
	}
	if err := store.Append(entry); err != nil {
		log.Printf("rdq exec: history append failed: %v", err)
	}
}

// renderResult writes the query result in the requested format. Table and
// JSON go to stdout; write-shape results (Updated >= 0) get a
// "(N rows affected)" note on stderr so stdout stays a clean data stream
// for piping.
func renderResult(stdout, stderr io.Writer, res *runner.Result, format string) error {
	if res == nil {
		// Defensive: runner.ExecuteSQL always returns a Result on success
		// but a future refactor might let this slip; a nil result is a
		// silent no-op rather than a panic.
		return nil
	}
	switch format {
	case "json":
		fmt.Fprintln(stdout, res.ToJSON())
	case "csv":
		if err := res.WriteCSV(stdout); err != nil {
			return err
		}
	case defaultOutputTable, "":
		if err := runner.RenderTable(stdout, res); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
	if res.Updated >= 0 {
		fmt.Fprintf(stderr, "(%d %s affected)\n", res.Updated, rowsWord(res.Updated))
	}
	return nil
}

func rowsWord(n int64) string {
	if n == 1 {
		return "row"
	}
	return "rows"
}
