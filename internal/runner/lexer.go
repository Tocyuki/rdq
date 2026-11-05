package runner

import "strings"

// This file holds the tiny SQL lexer shared by the read-only classifier,
// the destructive-statement gate, and the WITH splitter. It is not a full
// parser: it only knows enough to skip comments and quoted tokens so that
// keyword / punctuation searches are not fooled by their contents.

// skipSQLNoiseAt reports whether a comment or quoted token starts at
// sql[i] and, if so, returns the index just past it. Unterminated tokens
// run to len(sql) and report terminated=false so strict callers (e.g. the
// balanced-paren scanner) can fail closed.
//
// Recognized forms:
//   - `-- line comment`
//   - `/* block comment */` (no nesting)
//   - 'single-quoted string' with ” escape
//   - "double-quoted identifier" with "" escape
//   - `backtick identifier` (MySQL) with “ escape
//   - $$dollar-quoted$$ / $tag$dollar-quoted$tag$ strings (PostgreSQL)
//
// Backslash escapes inside single quotes are intentionally NOT handled:
// PostgreSQL's default standard_conforming_strings treats `\` literally,
// and the gates do not know the dialect.
func skipSQLNoiseAt(sql string, i int) (next int, skipped, terminated bool) {
	if i >= len(sql) {
		return i, false, true
	}
	switch c := sql[i]; {
	case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
		// The newline itself is left in place: it is whitespace, and
		// keeping it preserves line structure in stripSQLNoise output.
		j := strings.IndexByte(sql[i:], '\n')
		if j < 0 {
			return len(sql), true, true
		}
		return i + j, true, true
	case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
		j := strings.Index(sql[i+2:], "*/")
		if j < 0 {
			return len(sql), true, false
		}
		return i + 2 + j + 2, true, true
	case c == '\'' || c == '"' || c == '`':
		next, ok := skipSQLQuoted(sql, i, c)
		if !ok {
			return len(sql), true, false
		}
		return next, true, true
	case c == '$':
		tag, ok := dollarQuoteTag(sql, i)
		if !ok {
			return i, false, true
		}
		j := strings.Index(sql[i+len(tag):], tag)
		if j < 0 {
			return len(sql), true, false
		}
		return i + len(tag) + j + len(tag), true, true
	}
	return i, false, true
}

// skipSQLQuoted skips a token delimited by quote (doubling the quote is
// the escape). Returns the index just past the closing quote.
func skipSQLQuoted(sql string, i int, quote byte) (int, bool) {
	if i >= len(sql) || sql[i] != quote {
		return 0, false
	}
	i++
	for i < len(sql) {
		if sql[i] == quote {
			if i+1 < len(sql) && sql[i+1] == quote {
				i += 2
				continue
			}
			return i + 1, true
		}
		i++
	}
	return 0, false
}

// dollarQuoteTag returns the opening `$tag$` delimiter at sql[i], if any.
// `$1` style bind placeholders and `$` embedded inside an identifier
// (e.g. `foo$bar`) are not dollar quotes.
func dollarQuoteTag(sql string, i int) (string, bool) {
	if i >= len(sql) || sql[i] != '$' {
		return "", false
	}
	if i > 0 && isSQLIdentByte(sql[i-1]) {
		return "", false
	}
	j := i + 1
	if j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
		return "", false
	}
	for j < len(sql) && isSQLWordByte(sql[j]) {
		j++
	}
	if j < len(sql) && sql[j] == '$' {
		return sql[i : j+1], true
	}
	return "", false
}

// splitSQLStatements splits sql on `;` that are outside comments and
// quoted tokens. Empty / comment-only pieces are kept so callers can
// decide how strict to be about them.
func splitSQLStatements(sql string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(sql); {
		if next, skipped, _ := skipSQLNoiseAt(sql, i); skipped {
			i = next
			continue
		}
		if sql[i] == ';' {
			parts = append(parts, sql[start:i])
			start = i + 1
		}
		i++
	}
	return append(parts, sql[start:])
}

// isBlankSQL reports whether sql contains nothing but whitespace and
// comments.
func isBlankSQL(sql string) bool {
	return strings.TrimSpace(stripSQLNoise(sql)) == ""
}

// stripSQLNoise replaces comments and quoted tokens with a single space so
// keyword searches see only the statement's own tokens (and adjacent
// tokens never fuse into a new word).
func stripSQLNoise(sql string) string {
	var out strings.Builder
	out.Grow(len(sql))
	for i := 0; i < len(sql); {
		if next, skipped, _ := skipSQLNoiseAt(sql, i); skipped {
			out.WriteByte(' ')
			i = next
			continue
		}
		out.WriteByte(sql[i])
		i++
	}
	return out.String()
}

// hasTopLevelKeyword reports whether keyword (upper case) appears as a
// standalone word at parenthesis depth 0, outside comments and quoted
// tokens. Case is ignored. Keywords inside subqueries do not count, so
// `DELETE FROM t USING (SELECT ... WHERE ...) s` has no top-level WHERE.
func hasTopLevelKeyword(sql, keyword string) bool {
	upper := strings.ToUpper(stripSQLNoise(sql))
	n := len(keyword)
	depth := 0
	for i := 0; i < len(upper); i++ {
		switch upper[i] {
		case '(':
			depth++
			continue
		case ')':
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth != 0 || i+n > len(upper) || upper[i:i+n] != keyword {
			continue
		}
		if isSQLWordByte(byteAt(upper, i-1)) || isSQLWordByte(byteAt(upper, i+n)) {
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

// nonBlankStatements returns the `;`-separated statements in sql with
// whitespace / comment-only pieces dropped.
func nonBlankStatements(sql string) []string {
	var out []string
	for _, part := range splitSQLStatements(sql) {
		if !isBlankSQL(part) {
			out = append(out, part)
		}
	}
	return out
}
