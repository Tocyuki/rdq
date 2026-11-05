package runner

import "strings"

// This file holds the tiny SQL lexer shared by the read-only classifier,
// the destructive-statement gate, and the WITH splitter. It is not a full
// parser: it only knows enough to skip comments and quoted tokens so that
// keyword / punctuation searches are not fooled by their contents.
//
// Aurora PostgreSQL and Aurora MySQL disagree on several lexical rules,
// and the gates do not know which engine a statement is headed for. If the
// lexer followed one dialect, SQL could be crafted so that what the gate
// skips as a comment or string is executable code on the other engine
// (e.g. MySQL's `/*! ... */` executable comments, or `'a\''` which is a
// complete string in MySQL but an unterminated one in PostgreSQL). So every
// gate evaluates the statement once per dialect and combines the results
// conservatively: read-only only if read-only under every dialect, needs
// confirmation if it does under any dialect.

// dialect selects the lexical rules used to skip comments and quotes.
type dialect int

const (
	// dialectPostgres: `--` always starts a comment, '' is the only
	// escape in standard strings (E'...' also honours backslashes),
	// "..." is an identifier, and $tag$...$tag$ is a string.
	dialectPostgres dialect = iota
	// dialectMySQL: backslash escapes in '...' and "..." (both strings
	// under the default sql_mode), `#` comments, `--` is a comment only
	// when followed by whitespace, `/*! ... */` bodies are executed, and
	// there are no dollar quotes.
	dialectMySQL
)

// allDialects is the set every public gate evaluates against.
var allDialects = []dialect{dialectPostgres, dialectMySQL}

// skipSQLCommentAt reports whether a comment starts at sql[i] under d and,
// if so, returns the index just past it. For MySQL executable comments
// (`/*!` / `/*M!`, optionally followed by a version number) only the
// opening marker is consumed, so the body is lexed as code — MySQL runs
// it. Unterminated block comments report terminated=false.
func skipSQLCommentAt(d dialect, sql string, i int) (next int, skipped, terminated bool) {
	if i >= len(sql) {
		return i, false, true
	}
	rest := sql[i:]
	switch {
	case strings.HasPrefix(rest, "--"):
		if d == dialectMySQL && len(rest) > 2 && !isSQLCommentSpace(rest[2]) {
			// MySQL: `--1` is "minus minus 1", not a comment.
			return i, false, true
		}
		return skipToLineEnd(sql, i), true, true
	case d == dialectMySQL && rest[0] == '#':
		return skipToLineEnd(sql, i), true, true
	case strings.HasPrefix(rest, "/*"):
		if d == dialectMySQL {
			marker := 0
			switch {
			case strings.HasPrefix(rest, "/*!"):
				marker = 3
			case strings.HasPrefix(rest, "/*M!"):
				marker = 4
			}
			if marker > 0 {
				j := i + marker
				for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
					j++
				}
				return j, true, true
			}
		}
		j := strings.Index(rest[2:], "*/")
		if j < 0 {
			return len(sql), true, false
		}
		return i + 2 + j + 2, true, true
	}
	return i, false, true
}

// skipToLineEnd returns the index of the newline ending the line comment
// at sql[i] (or len(sql)). The newline itself is left in place: it is
// whitespace, and keeping it preserves line structure in stripSQLNoise.
func skipToLineEnd(sql string, i int) int {
	if j := strings.IndexByte(sql[i:], '\n'); j >= 0 {
		return i + j
	}
	return len(sql)
}

// isSQLCommentSpace is MySQL's rule for what may follow `--` in a comment.
func isSQLCommentSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v' || b < 0x20
}

// skipSQLNoiseAt reports whether a comment or quoted token starts at
// sql[i] under d and, if so, returns the index just past it. Unterminated
// tokens run to len(sql) and report terminated=false so strict callers
// (e.g. the balanced-paren scanner) can fail closed.
//
// Recognized forms (see dialect for which apply where):
//   - comments: `-- line`, `# line`, `/* block */` (no nesting)
//   - 'single-quoted string' with ” escape (plus backslash escapes in
//     MySQL and in PostgreSQL E'...' strings)
//   - "double-quoted" identifier (PostgreSQL) / string (MySQL)
//   - `backtick identifier` with “ escape
//   - $$dollar-quoted$$ / $tag$dollar-quoted$tag$ strings (PostgreSQL)
func skipSQLNoiseAt(d dialect, sql string, i int) (next int, skipped, terminated bool) {
	if next, skipped, terminated := skipSQLCommentAt(d, sql, i); skipped {
		return next, true, terminated
	}
	if i >= len(sql) {
		return i, false, true
	}
	switch c := sql[i]; c {
	case '\'', '"', '`':
		backslash := false
		switch {
		case d == dialectMySQL:
			backslash = c != '`'
		case c == '\'':
			backslash = isPostgresEscapeString(sql, i)
		}
		next, ok := skipSQLQuoted(sql, i, c, backslash)
		if !ok {
			return len(sql), true, false
		}
		return next, true, true
	case '$':
		if d != dialectPostgres {
			return i, false, true
		}
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

// isPostgresEscapeString reports whether the quote at sql[i] opens a
// PostgreSQL E'...' escape string (the E must be its own token, not the
// tail of an identifier such as `name'`).
func isPostgresEscapeString(sql string, i int) bool {
	if i < 1 || (sql[i-1] != 'E' && sql[i-1] != 'e') {
		return false
	}
	return i < 2 || !isSQLIdentByte(sql[i-2])
}

// skipSQLQuoted skips a token delimited by quote. Doubling the quote is
// always an escape; with backslash=true a backslash also escapes the next
// byte. Returns the index just past the closing quote.
func skipSQLQuoted(sql string, i int, quote byte, backslash bool) (int, bool) {
	if i >= len(sql) || sql[i] != quote {
		return 0, false
	}
	i++
	for i < len(sql) {
		switch {
		case backslash && sql[i] == '\\':
			i += 2
			continue
		case sql[i] == quote:
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
func splitSQLStatements(d dialect, sql string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(sql); {
		if next, skipped, _ := skipSQLNoiseAt(d, sql, i); skipped {
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
func isBlankSQL(d dialect, sql string) bool {
	return strings.TrimSpace(stripSQLNoise(d, sql)) == ""
}

// nonBlankStatements returns the `;`-separated statements in sql with
// whitespace / comment-only pieces dropped.
func nonBlankStatements(d dialect, sql string) []string {
	var out []string
	for _, part := range splitSQLStatements(d, sql) {
		if !isBlankSQL(d, part) {
			out = append(out, part)
		}
	}
	return out
}

// stripSQLNoise replaces comments and quoted tokens with a single space so
// keyword searches see only the statement's own tokens (and adjacent
// tokens never fuse into a new word).
func stripSQLNoise(d dialect, sql string) string {
	var out strings.Builder
	out.Grow(len(sql))
	for i := 0; i < len(sql); {
		if next, skipped, _ := skipSQLNoiseAt(d, sql, i); skipped {
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
func hasTopLevelKeyword(d dialect, sql, keyword string) bool {
	upper := strings.ToUpper(stripSQLNoise(d, sql))
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
