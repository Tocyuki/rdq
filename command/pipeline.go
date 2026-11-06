package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Tocyuki/rdq/internal/runner"
	"github.com/Tocyuki/rdq/internal/state"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// sqlPipeline is the downstream half shared by `rdq exec` and `rdq ask`:
// read-only gate → destructive confirmation → Data API → history → render.
// Keeping one implementation guarantees both commands apply identical
// policies and exit codes (and match the gates the TUI applies).
type sqlPipeline struct {
	name   string // stderr prefix, e.g. "rdq exec"
	yes    bool   // --yes: skip destructive confirmation
	output string // --output format

	executeSQL func(ctx context.Context, cfg aws.Config, target runner.Target, sql string) (*runner.Result, time.Duration, error)
	loadState  func() (*state.State, error)
	isTerminal func(io.Reader) bool
}

// run executes sql and returns the process exit code. sql must already be
// non-empty; input validation is the caller's job.
func (p sqlPipeline) run(globals *Globals, sql string, stdin io.Reader, stdout, stderr io.Writer) int {
	errorf := func(msg string) { fmt.Fprintln(stderr, p.name+": "+msg) }

	if isReadOnlyProfile(p.loadState, globals.Profile) && !runner.IsReadOnlySQL(sql) {
		errorf(runner.ErrWriteBlocked.Error())
		errorf("disable read-only mode in the TUI (F8), then retry.")
		recordHistory(p.name, globals, sql, runner.ErrWriteBlocked, 0)
		return exitReadOnly
	}

	if need, reason := runner.NeedsConfirmation(sql); need && !p.yes {
		if !p.isTerminal(stdin) {
			errorf("destructive statement requires --yes in non-interactive mode.")
			errorf(reason)
			return exitNotConfirmed
		}
		if !promptConfirm(p.name, stdin, stderr, reason) {
			errorf("aborted by user.")
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
	res, elapsed, execErr := p.executeSQL(ctx, globals.AWSConfig, target, sql)
	recordHistory(p.name, globals, sql, execErr, elapsed)

	if execErr != nil {
		errorf(execErr.Error())
		switch {
		case errors.Is(execErr, context.DeadlineExceeded):
			return exitTimeout
		case errors.Is(execErr, runner.ErrEmptySQL):
			return exitUsage
		default:
			return exitError
		}
	}

	if err := renderResult(stdout, stderr, res, p.output); err != nil {
		errorf(err.Error())
		return exitError
	}
	return exitSuccess
}
