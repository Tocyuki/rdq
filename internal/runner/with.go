package runner

import "strings"

// splitLeadingWith extracts the CTE bodies and trailing main statement from a
// leading WITH query. It understands the common PostgreSQL/Aurora forms we
// emit and inspect (optional RECURSIVE, optional column lists, optional
// MATERIALIZED / NOT MATERIALIZED). If parsing fails, ok=false and callers
// should fall back to the conservative path.
func splitLeadingWith(d dialect, sql string) (cteBodies []string, tail string, ok bool) {
	i := skipSQLSpaceAndComments(d, sql, 0)
	next, ok := consumeSQLKeyword(d, sql, i, "WITH")
	if !ok {
		return nil, "", false
	}
	i = skipSQLSpaceAndComments(d, sql, next)
	if next, ok := consumeSQLKeyword(d, sql, i, "RECURSIVE"); ok {
		i = next
	}

	for {
		i = skipSQLSpaceAndComments(d, sql, i)
		next, ok := readSQLIdentifier(d, sql, i)
		if !ok {
			return nil, "", false
		}
		i = skipSQLSpaceAndComments(d, sql, next)

		// Optional column list after the CTE name.
		if i < len(sql) && sql[i] == '(' {
			end, ok := scanSQLBalanced(d, sql, i)
			if !ok {
				return nil, "", false
			}
			i = skipSQLSpaceAndComments(d, sql, end+1)
		}

		next, ok = consumeSQLKeyword(d, sql, i, "AS")
		if !ok {
			return nil, "", false
		}
		i = skipSQLSpaceAndComments(d, sql, next)

		if next, ok := consumeSQLKeyword(d, sql, i, "NOT"); ok {
			i = skipSQLSpaceAndComments(d, sql, next)
			next, ok = consumeSQLKeyword(d, sql, i, "MATERIALIZED")
			if !ok {
				return nil, "", false
			}
			i = skipSQLSpaceAndComments(d, sql, next)
		} else if next, ok := consumeSQLKeyword(d, sql, i, "MATERIALIZED"); ok {
			i = skipSQLSpaceAndComments(d, sql, next)
		}

		if i >= len(sql) || sql[i] != '(' {
			return nil, "", false
		}
		end, ok := scanSQLBalanced(d, sql, i)
		if !ok {
			return nil, "", false
		}
		cteBodies = append(cteBodies, sql[i+1:end])
		i = skipSQLSpaceAndComments(d, sql, end+1)

		if i < len(sql) && sql[i] == ',' {
			i++
			continue
		}
		return cteBodies, sql[i:], true
	}
}

func skipSQLSpaceAndComments(d dialect, sql string, i int) int {
	for i < len(sql) {
		if isSQLSpace(sql[i]) {
			i++
			continue
		}
		next, skipped, terminated := skipSQLCommentAt(d, sql, i)
		if !skipped {
			return i
		}
		if !terminated {
			return len(sql)
		}
		i = next
	}
	return i
}

func readSQLIdentifier(d dialect, sql string, i int) (int, bool) {
	i = skipSQLSpaceAndComments(d, sql, i)
	if i >= len(sql) {
		return 0, false
	}
	if sql[i] == '"' || sql[i] == '`' {
		return skipSQLQuoted(sql, i, sql[i], false)
	}
	j := i
	for j < len(sql) && isSQLIdentByte(sql[j]) {
		j++
	}
	return j, j > i
}

func consumeSQLKeyword(d dialect, sql string, i int, kw string) (int, bool) {
	i = skipSQLSpaceAndComments(d, sql, i)
	j := i
	for j < len(sql) && isSQLWordByte(sql[j]) {
		j++
	}
	if j == i || !strings.EqualFold(sql[i:j], kw) {
		return 0, false
	}
	if j < len(sql) && isSQLWordByte(sql[j]) {
		return 0, false
	}
	return j, true
}

// scanSQLBalanced returns the index of the `)` matching the `(` at
// sql[start], skipping comments and quoted tokens. Unterminated input
// returns ok=false so callers fail closed.
func scanSQLBalanced(d dialect, sql string, start int) (int, bool) {
	if start >= len(sql) || sql[start] != '(' {
		return 0, false
	}
	depth := 0
	for i := start; i < len(sql); {
		if next, skipped, terminated := skipSQLNoiseAt(d, sql, i); skipped {
			if !terminated {
				return 0, false
			}
			i = next
			continue
		}
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
		i++
	}
	return 0, false
}

func isSQLSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
}

func isSQLIdentByte(b byte) bool {
	return isSQLWordByte(b) || b == '$'
}

func isSQLWordByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') ||
		(b >= 'a' && b <= 'z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}
