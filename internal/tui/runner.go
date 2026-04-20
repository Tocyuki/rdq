package tui

import (
	"context"
	"time"

	"github.com/Tocyuki/rdq/internal/runner"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	tea "github.com/charmbracelet/bubbletea"
)

// target captures the RDS Data API connection coordinates resolved by the
// command layer. The TUI does not care how the user got here, only what to
// pass to runner.ExecuteSQL.
type target struct {
	profile  string
	region   string
	cluster  string
	secret   string
	database string
}

// toRunnerTarget converts the TUI's internal target to runner.Target. Doing
// the conversion at a single call site keeps the TUI's connection state names
// stable across the many files that reference them.
func (t target) toRunnerTarget() runner.Target {
	return runner.Target{
		Profile:  t.profile,
		Region:   t.region,
		Cluster:  t.cluster,
		Secret:   t.secret,
		Database: t.database,
	}
}

// executeMsg is sent when an SQL execution finishes (success or failure).
// SQL is captured at run time so the history layer can record the exact
// statement even if the editor has changed by the time the result arrives.
type executeMsg struct {
	SQL      string
	Result   *runner.Result
	Err      error
	Duration time.Duration
}

// runStatement returns a tea.Cmd that invokes runner.ExecuteSQL and emits an
// executeMsg with the result. Empty SQL returns runner.ErrEmptySQL so the
// View layer can treat it as a hint rather than a real error.
//
// readOnly gates destructive statements: when true and the SQL's leading
// keyword is not a pure read, the command short-circuits with
// runner.ErrWriteBlocked so no AWS round trip happens. The caller
// (Model.readOnlyForRun) decides the policy by consulting state.json,
// keeping the TUI consistent with the GUI's /api/execute enforcement.
func runStatement(client *rdsdata.Client, tgt target, sql string, readOnly bool) tea.Cmd {
	return func() tea.Msg {
		if readOnly && !runner.IsReadOnlySQL(sql) {
			return executeMsg{SQL: sql, Err: runner.ErrWriteBlocked}
		}
		ctx, cancel := context.WithTimeout(context.Background(), runner.ExecuteTimeout)
		defer cancel()

		res, elapsed, err := runner.ExecuteSQL(ctx, client, tgt.toRunnerTarget(), sql)
		if err != nil {
			return executeMsg{SQL: sql, Err: err, Duration: elapsed}
		}
		return executeMsg{SQL: sql, Result: res, Duration: elapsed}
	}
}
