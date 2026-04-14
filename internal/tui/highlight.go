package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// highlightSQL renders SQL with monokai colors as ANSI escape sequences. The
// terminal256 formatter is used for broad terminal compatibility (true color
// looks slightly nicer but is not universal). On failure the original string
// is returned so the editor preview never goes blank.
func highlightSQL(code string) string {
	return highlight(code, "sql")
}

// highlightJSON renders JSON with monokai colors. Used for the table → JSON
// view and the row inspector so number / string / boolean values jump out.
func highlightJSON(code string) string {
	return highlight(code, "json")
}

func highlight(code, lang string) string {
	if code == "" {
		return ""
	}
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var buf strings.Builder
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return code
	}
	return buf.String()
}

// shortenModelID compacts a Bedrock model identifier or inference profile ID
// down to its most informative slug so the status bar stays readable.
//
// Examples:
//
//	"us.anthropic.claude-sonnet-4-5-20250929-v1:0" -> "claude-sonnet-4-5-20250929"
//	"anthropic.claude-3-5-sonnet-20241022-v2:0"    -> "claude-3-5-sonnet-20241022"
//	"meta.llama3-70b-instruct-v1:0"                -> "llama3-70b-instruct"
func shortenModelID(id string) string {
	if id == "" {
		return ""
	}
	s := id
	// Drop the trailing version suffix ("-v1:0", "-v2:0", etc.).
	if idx := strings.LastIndex(s, "-v"); idx > 0 {
		// Only drop if what follows looks like a version: digit + optional ":N"
		tail := s[idx+2:]
		if isVersionTail(tail) {
			s = s[:idx]
		}
	}
	// Strip the provider prefix and any inference profile region prefix.
	// "us.anthropic.claude-..." -> "claude-..."
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[i+1:]
	}
	return s
}

func isVersionTail(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			continue
		}
		if c == ':' || c == '.' {
			continue
		}
		return false
	}
	return true
}

// padToHeight ensures s has exactly height visible lines, trimming or padding
// with empty lines so the editor preview keeps the same vertical footprint as
// the underlying textarea.
func padToHeight(s string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
