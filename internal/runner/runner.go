// Package runner executes SQL statements against the AWS RDS Data API and
// shapes the response into a UI-friendly Result. It is deliberately free of
// any terminal/bubbletea or HTTP concerns so the TUI, CLI, and GUI can all
// reuse the same engine.
package runner

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
)

// Target captures the RDS Data API connection coordinates resolved by the
// caller (command layer for TUI, HTTP handler for GUI).
type Target struct {
	Profile  string
	Region   string
	Cluster  string
	Secret   string
	Database string
}

// ExecuteTimeout caps a single statement so a runaway query does not lock up
// the caller. The Data API itself enforces shorter limits, so this is a safety
// net rather than the primary bound. The caller is free to impose a stricter
// ctx deadline.
const ExecuteTimeout = 2 * time.Minute

// ErrEmptySQL is returned when the SQL text is empty or whitespace only. Callers
// should treat it as a user-facing hint rather than a real error.
var ErrEmptySQL = errors.New("enter a SQL statement to run")

// ExecuteSQL runs the given SQL against the Data API and returns a parsed
// Result. It is a synchronous, side-effect free function so HTTP handlers and
// tea.Cmd wrappers can share it. The caller is responsible for history logging.
func ExecuteSQL(ctx context.Context, client *rdsdata.Client, target Target, sql string) (*Result, time.Duration, error) {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return nil, 0, ErrEmptySQL
	}
	start := time.Now()
	out, err := client.ExecuteStatement(ctx, &rdsdata.ExecuteStatementInput{
		ResourceArn:           aws.String(target.Cluster),
		SecretArn:             aws.String(target.Secret),
		Database:              aws.String(target.Database),
		Sql:                   aws.String(trimmed),
		IncludeResultMetadata: true,
	})
	elapsed := time.Since(start)
	if err != nil {
		return nil, elapsed, err
	}
	return ConvertResult(out), elapsed, nil
}
