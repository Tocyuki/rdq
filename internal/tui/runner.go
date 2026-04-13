package tui

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	tea "github.com/charmbracelet/bubbletea"
)

// target captures the RDS Data API connection coordinates resolved by the
// command layer. The TUI does not care how the user got here, only what to
// pass to ExecuteStatement.
type target struct {
	profile  string
	region   string
	cluster  string
	secret   string
	database string
}

// executeMsg is sent when an SQL execution finishes (success or failure).
type executeMsg struct {
	Result   *queryResult
	Err      error
	Duration time.Duration
}

// runStatement returns a tea.Cmd that invokes the RDS Data API and emits an
// executeMsg with the result. Empty/whitespace SQL becomes a no-op error so
// the user gets immediate feedback without a round trip.
func runStatement(client *rdsdata.Client, target target, sql string) tea.Cmd {
	return func() tea.Msg {
		trimmed := strings.TrimSpace(sql)
		if trimmed == "" {
			return executeMsg{Err: errEmptySQL}
		}
		ctx, cancel := context.WithTimeout(context.Background(), executeTimeout)
		defer cancel()

		start := time.Now()
		out, err := client.ExecuteStatement(ctx, &rdsdata.ExecuteStatementInput{
			ResourceArn:           aws.String(target.cluster),
			SecretArn:             aws.String(target.secret),
			Database:              aws.String(target.database),
			Sql:                   aws.String(trimmed),
			IncludeResultMetadata: true,
		})
		elapsed := time.Since(start)
		if err != nil {
			return executeMsg{Err: err, Duration: elapsed}
		}
		return executeMsg{Result: convertResult(out), Duration: elapsed}
	}
}

// executeTimeout caps a single statement so a runaway query does not lock up
// the TUI indefinitely. The Data API itself has shorter limits, so this is a
// safety net rather than the primary bound.
const executeTimeout = 2 * time.Minute

// errEmptySQL is returned to the UI when the user presses run with an empty
// editor. It is a sentinel so the View layer can treat it as a hint rather
// than a real error.
var errEmptySQL = errEmptySQLValue{}

type errEmptySQLValue struct{}

func (errEmptySQLValue) Error() string { return "enter a SQL statement to run" }
