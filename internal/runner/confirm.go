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
func NeedsConfirmation(sql string) (bool, string) {
	head := firstKeyword(sql)
	switch head {
	case "WITH":
		bodies, tail, ok := splitLeadingWith(sql)
		if !ok || len(bodies) == 0 {
			return true, "WITH statements can hide destructive writes; review this query before running it."
		}
		for _, body := range bodies {
			if need, reason := NeedsConfirmation(body); need {
				return true, reason
			}
		}
		if strings.TrimSpace(tail) == "" {
			return true, "WITH statements can hide destructive writes; review this query before running it."
		}
		return NeedsConfirmation(tail)
	case "TRUNCATE":
		return true, "TRUNCATE removes every row from the table — this is not reversible."
	case "DELETE":
		if !hasWhereClause(sql) {
			return true, "DELETE without a WHERE clause will remove every row in the table."
		}
	case "UPDATE":
		if !hasWhereClause(sql) {
			return true, "UPDATE without a WHERE clause will rewrite every row in the table."
		}
	}
	return false, ""
}

// hasWhereClause returns true when sql contains a WHERE keyword that is
// *not* inside a comment or string literal. Case is ignored.
func hasWhereClause(sql string) bool {
	stripped := stripSQLNoise(sql)
	upper := strings.ToUpper(stripped)
	return containsKeyword(upper, "WHERE")
}

// stripSQLNoise removes line comments, block comments, single-quoted
// strings, and double-quoted identifiers from sql. It is a naive
// lexer, not a full SQL parser — enough to avoid the common false
// positives on WHERE detection without pulling in an external
// dependency.
func stripSQLNoise(sql string) string {
	var out strings.Builder
	out.Grow(len(sql))
	i := 0
	for i < len(sql) {
		// -- line comment
		if i+1 < len(sql) && sql[i] == '-' && sql[i+1] == '-' {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		// /* ... */ block comment (no nesting — standard SQL)
		if i+1 < len(sql) && sql[i] == '/' && sql[i+1] == '*' {
			i += 2
			for i+1 < len(sql) && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			if i+1 < len(sql) {
				i += 2
			} else {
				i = len(sql)
			}
			continue
		}
		// 'single-quoted string' with '' escape
		if sql[i] == '\'' {
			i++
			for i < len(sql) {
				if sql[i] == '\'' {
					if i+1 < len(sql) && sql[i+1] == '\'' {
						i += 2 // escaped quote
						continue
					}
					i++
					break
				}
				i++
			}
			continue
		}
		// "double-quoted identifier"
		if sql[i] == '"' {
			i++
			for i < len(sql) && sql[i] != '"' {
				i++
			}
			if i < len(sql) {
				i++
			}
			continue
		}
		out.WriteByte(sql[i])
		i++
	}
	return out.String()
}

// containsKeyword returns true when upper contains keyword as a standalone
// ASCII-alphanumeric word (i.e. bounded on both sides by non-word bytes or
// start/end of string). Reimplemented here instead of reaching for regexp
// so the detector has zero non-stdlib dependencies at this hot path.
func containsKeyword(upper, keyword string) bool {
	n := len(keyword)
	for i := 0; i+n <= len(upper); i++ {
		if upper[i:i+n] != keyword {
			continue
		}
		if isWordByte(byteAt(upper, i-1)) || isWordByte(byteAt(upper, i+n)) {
			continue
		}
		return true
	}
	return false
}

func byteAt(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

func isWordByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') ||
		(b >= 'a' && b <= 'z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}
