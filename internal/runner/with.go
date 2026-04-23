package runner

import "strings"

// splitLeadingWith extracts the CTE bodies and trailing main statement from a
// leading WITH query. It understands the common PostgreSQL/Aurora forms we
// emit and inspect (optional RECURSIVE, optional column lists, optional
// MATERIALIZED / NOT MATERIALIZED). If parsing fails, ok=false and callers
// should fall back to the conservative path.
func splitLeadingWith(sql string) (cteBodies []string, tail string, ok bool) {
	i := skipSQLSpaceAndComments(sql, 0)
	next, ok := consumeSQLKeyword(sql, i, "WITH")
	if !ok {
		return nil, "", false
	}
	i = skipSQLSpaceAndComments(sql, next)
	if next, ok := consumeSQLKeyword(sql, i, "RECURSIVE"); ok {
		i = next
	}

	for {
		i = skipSQLSpaceAndComments(sql, i)
		next, ok := readSQLIdentifier(sql, i)
		if !ok {
			return nil, "", false
		}
		i = skipSQLSpaceAndComments(sql, next)

		// Optional column list after the CTE name.
		if i < len(sql) && sql[i] == '(' {
			end, ok := scanSQLBalanced(sql, i)
			if !ok {
				return nil, "", false
			}
			i = skipSQLSpaceAndComments(sql, end+1)
		}

		next, ok = consumeSQLKeyword(sql, i, "AS")
		if !ok {
			return nil, "", false
		}
		i = skipSQLSpaceAndComments(sql, next)

		if next, ok := consumeSQLKeyword(sql, i, "NOT"); ok {
			i = skipSQLSpaceAndComments(sql, next)
			next, ok = consumeSQLKeyword(sql, i, "MATERIALIZED")
			if !ok {
				return nil, "", false
			}
			i = skipSQLSpaceAndComments(sql, next)
		} else if next, ok := consumeSQLKeyword(sql, i, "MATERIALIZED"); ok {
			i = skipSQLSpaceAndComments(sql, next)
		}

		if i >= len(sql) || sql[i] != '(' {
			return nil, "", false
		}
		end, ok := scanSQLBalanced(sql, i)
		if !ok {
			return nil, "", false
		}
		cteBodies = append(cteBodies, sql[i+1:end])
		i = skipSQLSpaceAndComments(sql, end+1)

		if i < len(sql) && sql[i] == ',' {
			i++
			continue
		}
		return cteBodies, sql[i:], true
	}
}

func skipSQLSpaceAndComments(sql string, i int) int {
	for i < len(sql) {
		switch {
		case strings.HasPrefix(sql[i:], "--"):
			i += 2
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
		case strings.HasPrefix(sql[i:], "/*"):
			i += 2
			for i+1 < len(sql) && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			if i+1 < len(sql) {
				i += 2
			} else {
				return len(sql)
			}
		case isSQLSpace(sql[i]):
			i++
		default:
			return i
		}
	}
	return i
}

func readSQLIdentifier(sql string, i int) (int, bool) {
	i = skipSQLSpaceAndComments(sql, i)
	if i >= len(sql) {
		return 0, false
	}
	if sql[i] == '"' {
		return skipSQLDoubleQuoted(sql, i)
	}
	j := i
	for j < len(sql) && isSQLIdentByte(sql[j]) {
		j++
	}
	return j, j > i
}

func consumeSQLKeyword(sql string, i int, kw string) (int, bool) {
	i = skipSQLSpaceAndComments(sql, i)
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

func scanSQLBalanced(sql string, start int) (int, bool) {
	if start >= len(sql) || sql[start] != '(' {
		return 0, false
	}
	depth := 0
	for i := start; i < len(sql); i++ {
		switch {
		case strings.HasPrefix(sql[i:], "--"):
			i += 2
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
		case strings.HasPrefix(sql[i:], "/*"):
			i += 2
			for i+1 < len(sql) && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			if i+1 >= len(sql) {
				return 0, false
			}
			i++
		case sql[i] == '\'':
			next, ok := skipSQLSingleQuoted(sql, i)
			if !ok {
				return 0, false
			}
			i = next - 1
		case sql[i] == '"':
			next, ok := skipSQLDoubleQuoted(sql, i)
			if !ok {
				return 0, false
			}
			i = next - 1
		case sql[i] == '(':
			depth++
		case sql[i] == ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func skipSQLSingleQuoted(sql string, i int) (int, bool) {
	if i >= len(sql) || sql[i] != '\'' {
		return 0, false
	}
	i++
	for i < len(sql) {
		if sql[i] == '\'' {
			if i+1 < len(sql) && sql[i+1] == '\'' {
				i += 2
				continue
			}
			return i + 1, true
		}
		i++
	}
	return 0, false
}

func skipSQLDoubleQuoted(sql string, i int) (int, bool) {
	if i >= len(sql) || sql[i] != '"' {
		return 0, false
	}
	i++
	for i < len(sql) {
		if sql[i] == '"' {
			if i+1 < len(sql) && sql[i+1] == '"' {
				i += 2
				continue
			}
			return i + 1, true
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
