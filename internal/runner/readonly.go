package runner

import (
	"errors"
	"strings"
	"unicode"
)

// ErrWriteBlocked is returned when a statement is rejected because the
// active connection is in read-only mode. Callers surface this to the UI
// with a hint that the read-only toggle in Settings can be flipped.
var ErrWriteBlocked = errors.New("read-only mode blocks non-SELECT statements")

// readOnlyPrefixes is the allow-list of leading keywords we consider safe
// under read-only mode. It is intentionally conservative: anything whose
// first keyword is not on this list is rejected.
//
// Notable exclusions:
//
//   - USE (MySQL): switches the current schema; harmless by itself but the
//     Data API ignores it (ResourceArn drives the connection), so there is
//     no reason to allow it.
//   - SET: some variants are side-effecting (SET autocommit, SET ROLE).
var readOnlyPrefixes = map[string]struct{}{
	"SELECT":   {},
	"SHOW":     {},
	"EXPLAIN":  {},
	"DESCRIBE": {},
	"DESC":     {},
	"TABLE":    {}, // PostgreSQL's TABLE t shorthand for SELECT * FROM t
	"VALUES":   {}, // standalone VALUES clause is SELECT-like
}

// IsReadOnlySQL reports whether the given SQL is a pure read operation
// based on its leading keyword. Line (`--`) and block (`/* */`) comments
// and surrounding whitespace are skipped before the keyword is extracted.
// Unknown / non-alphabetic leading tokens return false so the safe default
// is "reject".
//
// Additional fail-closed rules on top of the keyword allow-list:
//
//   - Every `;`-separated statement must be read-only (`SELECT 1; DELETE
//     FROM t` is rejected). A trailing `;` is fine, but an empty statement
//     before a real one (`;SELECT 1`) is rejected as malformed input.
//   - EXPLAIN is only read-only when the explained statement is, because
//     `EXPLAIN ANALYZE <stmt>` executes <stmt> on both PostgreSQL and
//     MySQL. As a consequence MySQL's `EXPLAIN <table>` shorthand is
//     rejected; use `DESCRIBE <table>` instead.
//   - SELECT ... INTO creates a table (PostgreSQL) or writes a file /
//     variables (MySQL), so a top-level INTO disqualifies a SELECT.
func IsReadOnlySQL(sql string) bool {
	parts := splitSQLStatements(sql)
	for len(parts) > 1 && isBlankSQL(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}
	for _, part := range parts {
		if !isReadOnlyStatement(part) {
			return false
		}
	}
	return true
}

func isReadOnlyStatement(sql string) bool {
	head := firstKeyword(sql)
	switch head {
	case "":
		return false
	case "WITH":
		bodies, tail, ok := splitLeadingWith(sql)
		if !ok || len(bodies) == 0 {
			return false
		}
		for _, body := range bodies {
			if !isReadOnlyStatement(body) {
				return false
			}
		}
		return isReadOnlyStatement(tail)
	case "EXPLAIN":
		rest, ok := explainTarget(sql)
		return ok && isReadOnlyStatement(rest)
	case "SELECT", "VALUES", "TABLE":
		if hasTopLevelKeyword(sql, "INTO") {
			return false
		}
	}
	_, ok := readOnlyPrefixes[head]
	return ok
}

// explainOptionWords are the bare option keywords that may sit between
// EXPLAIN and the explained statement (PostgreSQL legacy syntax and MySQL).
var explainOptionWords = []string{"ANALYZE", "ANALYSE", "VERBOSE", "EXTENDED", "PARTITIONS"}

// explainTarget strips the leading EXPLAIN keyword plus any option list
// (`(ANALYZE, BUFFERS)`, `ANALYZE VERBOSE`, `FORMAT=JSON`) and returns the
// statement being explained.
func explainTarget(sql string) (string, bool) {
	sql = strings.TrimPrefix(sql, "\uFEFF")
	i := skipSQLSpaceAndComments(sql, 0)
	next, ok := consumeSQLKeyword(sql, i, "EXPLAIN")
	if !ok {
		return "", false
	}
	i = skipSQLSpaceAndComments(sql, next)
	if i < len(sql) && sql[i] == '(' {
		end, ok := scanSQLBalanced(sql, i)
		if !ok {
			return "", false
		}
		i = end + 1
	}
	for {
		i = skipSQLSpaceAndComments(sql, i)
		if next, ok := consumeAnySQLKeyword(sql, i, explainOptionWords); ok {
			i = next
			continue
		}
		if next, ok := consumeSQLKeyword(sql, i, "FORMAT"); ok {
			i = skipSQLSpaceAndComments(sql, next)
			if i < len(sql) && sql[i] == '=' {
				i = skipSQLSpaceAndComments(sql, i+1)
			}
			j := i
			for j < len(sql) && isSQLWordByte(sql[j]) {
				j++
			}
			if j == i {
				return "", false
			}
			i = j
			continue
		}
		return sql[i:], true
	}
}

func consumeAnySQLKeyword(sql string, i int, kws []string) (int, bool) {
	for _, kw := range kws {
		if next, ok := consumeSQLKeyword(sql, i, kw); ok {
			return next, true
		}
	}
	return 0, false
}

// IsAutoRunnableSQL is stricter than IsReadOnlySQL because it gates SQL that
// may execute immediately after an AI response. EXPLAIN is intentionally
// excluded here: PostgreSQL EXPLAIN ANALYZE executes the underlying statement,
// and a broad keyword check cannot distinguish every dangerous shape safely.
func IsAutoRunnableSQL(sql string) bool {
	head := firstKeyword(sql)
	if head == "EXPLAIN" {
		return false
	}
	return IsReadOnlySQL(sql)
}

// firstKeyword returns the first alphabetic word in sql, uppercased, with
// leading whitespace and SQL comments stripped. Whitespace handling uses
// unicode.IsSpace so accidental NBSP / ideographic-space bytes pasted in
// from editors or docs do not cause the classifier to misidentify a
// SELECT as a write. Returns "" if no keyword can be identified.
func firstKeyword(sql string) string {
	remaining := sql
	// Strip a UTF-8 BOM if present. unicode.IsSpace does not treat BOM
	// as whitespace, but keyboards / pastes occasionally introduce it.
	remaining = strings.TrimPrefix(remaining, "\uFEFF")
	for {
		remaining = strings.TrimLeftFunc(remaining, unicode.IsSpace)
		if remaining == "" {
			return ""
		}
		if strings.HasPrefix(remaining, "--") {
			// Consume until newline.
			if idx := strings.IndexByte(remaining, '\n'); idx >= 0 {
				remaining = remaining[idx+1:]
				continue
			}
			return ""
		}
		if strings.HasPrefix(remaining, "/*") {
			if idx := strings.Index(remaining[2:], "*/"); idx >= 0 {
				remaining = remaining[2+idx+2:]
				continue
			}
			return ""
		}
		break
	}
	// Extract leading ASCII letter run.
	end := 0
	for end < len(remaining) {
		c := remaining[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return ""
	}
	return strings.ToUpper(remaining[:end])
}
