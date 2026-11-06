// Package runner executes SQL statements against the AWS RDS Data API and
// shapes the response into a UI-friendly Result. It is deliberately free of
// any terminal/bubbletea concerns so the TUI and CLI can both reuse the same
// engine.
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
// caller (the command layer for the TUI and CLI).
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

// ExecuteSQLWithConfig builds a Data API client from cfg and runs sql via
// ExecuteSQL. It is the production implementation behind the executeSQL
// seams in the CLI (command/).
func ExecuteSQLWithConfig(ctx context.Context, cfg aws.Config, target Target, sql string) (*Result, time.Duration, error) {
	return ExecuteSQL(ctx, rdsdata.NewFromConfig(cfg), target, sql)
}

// ExecuteSQL runs the given SQL against the Data API and returns a parsed
// Result. It is a synchronous, side-effect free function so CLI commands and
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
