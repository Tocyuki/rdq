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
	if head == "WITH" {
		bodies, tail, ok := splitLeadingWith(sql)
		if !ok || len(bodies) == 0 {
			return false
		}
		for _, body := range bodies {
			if !IsReadOnlySQL(body) {
				return false
			}
		}
		return IsReadOnlySQL(tail)
	}
	_, ok := readOnlyPrefixes[head]
	return ok
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
