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
//   - WITH: CTEs that end in DELETE/UPDATE/INSERT are writes on PostgreSQL.
//     We still allow WITH because the vast majority of analytical CTEs end
//     in SELECT; a read-only user who tries a data-modifying CTE gets the
//     AWS error back. Trade-off documented in the Settings page copy.
var readOnlyPrefixes = map[string]struct{}{
	"SELECT":   {},
	"WITH":     {},
	"SHOW":     {},
	"EXPLAIN":  {},
	"DESCRIBE": {},
	"DESC":     {},
	"TABLE":    {}, // PostgreSQL's TABLE t shorthand for SELECT * FROM t
	"VALUES":   {}, // standalone VALUES clause is SELECT-like
}

// IsReadOnlySQL reports whether the given SQL statement is a pure read
// operation based on its leading keyword. Line (`--`) and block (`/* */`)
// comments and surrounding whitespace are skipped before the keyword is
// extracted. Unknown / non-alphabetic leading tokens return false so the
// safe default is "reject".
func IsReadOnlySQL(sql string) bool {
	head := firstKeyword(sql)
	if head == "" {
		return false
	}
	_, ok := readOnlyPrefixes[head]
	return ok
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
