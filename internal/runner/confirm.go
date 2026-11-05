package runner

import (
	"strings"
)

// NeedsConfirmation reports whether a statement is destructive enough that
// the UI should require an extra "yes, run it" from the user before
// dispatching it to the Data API. It returns a short human-readable
// reason alongside the bool so the confirmation prompt / dialog can
// quote it verbatim.
//
// The rule is deliberately narrow so it doesn't nag the user on every
// write: we only warn on the combinations that overwhelmingly correlate
// with accidents.
//
//   - DELETE without a WHERE clause → nukes every row
//   - UPDATE without a WHERE clause → rewrites every row
//   - TRUNCATE (no WHERE clause even exists in the grammar) → drops all rows
//
// Comments and string literals are stripped before the WHERE search so
// `UPDATE t SET c = 'WHERE it failed'` and
// `DELETE FROM t -- no WHERE here` are not fooled.
//
// A request carrying several `;`-separated statements always needs
// confirmation: each statement is checked individually (so the most
// specific reason wins), and if none trips a rule on its own the batch
// still gets a generic warning because the classifier cannot reason about
// how the statements interact.
//
// The checks run once per SQL dialect (see lexer.go); confirmation is
// required if any dialect's reading of the statement requires it.
func NeedsConfirmation(sql string) (bool, string) {
	for _, d := range allDialects {
		if need, reason := needsConfirmationIn(d, sql); need {
			return true, reason
		}
	}
	return false, ""
}

func needsConfirmationIn(d dialect, sql string) (bool, string) {
	stmts := nonBlankStatements(d, sql)
	if len(stmts) <= 1 {
		return needsConfirmationSingle(d, sql)
	}
	for _, stmt := range stmts {
		if need, reason := needsConfirmationSingle(d, stmt); need {
			return true, reason
		}
	}
	return true, "Multiple statements in one request; review each of them before running."
}

func needsConfirmationSingle(d dialect, sql string) (bool, string) {
	head := firstKeyword(d, sql)
	switch head {
	case "WITH":
		bodies, tail, ok := splitLeadingWith(d, sql)
		if !ok || len(bodies) == 0 {
			return true, "WITH statements can hide destructive writes; review this query before running it."
		}
		for _, body := range bodies {
			if need, reason := needsConfirmationSingle(d, body); need {
				return true, reason
			}
		}
		if strings.TrimSpace(tail) == "" {
			return true, "WITH statements can hide destructive writes; review this query before running it."
		}
		return needsConfirmationSingle(d, tail)
	case "TRUNCATE":
		return true, "TRUNCATE removes every row from the table — this is not reversible."
	case "DELETE":
		if !hasWhereClause(d, sql) {
			return true, "DELETE without a WHERE clause will remove every row in the table."
		}
	case "UPDATE":
		if !hasWhereClause(d, sql) {
			return true, "UPDATE without a WHERE clause will rewrite every row in the table."
		}
	}
	return false, ""
}

// hasWhereClause returns true when sql contains a top-level WHERE keyword
// that is *not* inside a comment, string literal, or subquery. Case is
// ignored. A WHERE that only appears in a subquery (e.g.
// `UPDATE t SET c = (SELECT max(v) FROM u WHERE ...)`) does not restrict
// the outer statement, so it does not count.
func hasWhereClause(d dialect, sql string) bool {
	return hasTopLevelKeyword(d, sql, "WHERE")
}
