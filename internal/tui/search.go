package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/Tocyuki/rdq/internal/runner"
	"github.com/charmbracelet/lipgloss"
)

// tableHit locates a substring match inside the results table. A single cell
// can contain multiple hits, and one row can contribute several across cells.
type tableHit struct {
	row    int
	cell   int
	col    int // byte offset in the formatted cell string
	length int // byte length of the match in the cell (may differ from the query's)
}

// jsonHit locates a substring match inside the rendered JSON view. Offsets are
// against the plain (un-highlighted) jsonRaw text so they survive regardless
// of Chroma's ANSI decoration.
type jsonHit struct {
	line   int
	col    int
	length int
}

// searchMatchStyle highlights every hit in the current search. The current
// (cursor) hit uses searchCurrentStyle so the user can always tell where n/N
// will land next.
var (
	searchMatchStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("11")). // bright yellow
				Foreground(lipgloss.Color("0")).
				Bold(true)
	searchCurrentStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("13")). // bright magenta
				Foreground(lipgloss.Color("0")).
				Bold(true)
)

// findAllFold returns the [start, end) byte ranges of every
// non-overlapping, case-insensitive occurrence of needle in hay. Offsets
// index hay itself, so callers can slice the original text safely.
//
// strings.ToLower cannot be used to compute offsets because it is not
// length-preserving for every rune (e.g. 'İ' grows from 2 to 3 bytes, the
// Kelvin sign 'K' shrinks from 3 to 1), which would make offsets into the
// lowered copy slice the original out of range or mid-rune.
func findAllFold(hay, needle string) [][2]int {
	if needle == "" || hay == "" {
		return nil
	}
	if isASCII(hay) && isASCII(needle) {
		// Fast path: ASCII lower-casing preserves byte offsets.
		lowerHay, lowerNeedle := strings.ToLower(hay), strings.ToLower(needle)
		var out [][2]int
		for start := 0; ; {
			idx := strings.Index(lowerHay[start:], lowerNeedle)
			if idx < 0 {
				return out
			}
			begin := start + idx
			start = begin + len(lowerNeedle)
			out = append(out, [2]int{begin, start})
		}
	}
	n := utf8.RuneCountInString(needle)
	var out [][2]int
	for i := 0; i < len(hay); {
		end := i
		for k := 0; k < n && end < len(hay); k++ {
			_, size := utf8.DecodeRuneInString(hay[end:])
			end += size
		}
		if strings.EqualFold(hay[i:end], needle) {
			out = append(out, [2]int{i, end})
			i = end
			continue
		}
		_, size := utf8.DecodeRuneInString(hay[i:])
		i += size
	}
	return out
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// highlightMatches renders text with every match of query wrapped in the
// match style; the span for which isCurrent(start) is true uses the
// brighter current-hit style instead.
func highlightMatches(text, query string, isCurrent func(start int) bool) string {
	if query == "" || text == "" {
		return text
	}
	matches := findAllFold(text, query)
	if len(matches) == 0 {
		return text
	}
	var b strings.Builder
	prev := 0
	for _, mt := range matches {
		b.WriteString(text[prev:mt[0]])
		style := searchMatchStyle
		if isCurrent(mt[0]) {
			style = searchCurrentStyle
		}
		b.WriteString(style.Render(text[mt[0]:mt[1]]))
		prev = mt[1]
	}
	b.WriteString(text[prev:])
	return b.String()
}

// computeTableHits scans every cell in the result rows for case-insensitive
// occurrences of query. Rows are walked in display order so n/N cycles
// top-to-bottom, left-to-right across the full result set.
func computeTableHits(result *queryResult, query string) []tableHit {
	if result == nil || query == "" {
		return nil
	}
	var hits []tableHit
	for r, row := range result.Rows {
		for c := range result.Columns {
			var cell any
			if c < len(row) {
				cell = row[c]
			}
			for _, mt := range findAllFold(runner.FormatCell(cell), query) {
				hits = append(hits, tableHit{row: r, cell: c, col: mt[0], length: mt[1] - mt[0]})
			}
		}
	}
	return hits
}

// computeJSONHits scans jsonRaw line-by-line for case-insensitive occurrences
// of query.
func computeJSONHits(jsonRaw, query string) []jsonHit {
	if jsonRaw == "" || query == "" {
		return nil
	}
	var hits []jsonHit
	for i, line := range strings.Split(jsonRaw, "\n") {
		for _, mt := range findAllFold(line, query) {
			hits = append(hits, jsonHit{line: i, col: mt[0], length: mt[1] - mt[0]})
		}
	}
	return hits
}

// highlightCell wraps every occurrence of query inside text in the configured
// match style. When the cell contains the caller's "current" hit (identified
// by the row/cell/col triplet in cur), that specific span uses the brighter
// current style instead. Non-matching spans are returned verbatim so the
// caller can drop the result straight into a bubbles/table row.
//
// text is expected to already be the truncated cell value that will appear
// on screen. The search is repeated on the visible string rather than reusing
// the pre-computed table hit offsets because truncation can shift or drop
// occurrences near the right edge.
func highlightCell(text, query string, cur *tableHit, row, cell int) string {
	return highlightMatches(text, query, func(start int) bool {
		return cur != nil && cur.row == row && cur.cell == cell && cur.col == start
	})
}

// highlightJSONLine decorates one plain-text JSON line with match highlights.
// When the line holds the current cursor hit, that span uses the current
// style. Used only on lines where at least one match exists; untouched lines
// keep their Chroma colouring in refreshJSON.
func highlightJSONLine(line, query string, cur *jsonHit, lineIdx int) string {
	return highlightMatches(line, query, func(start int) bool {
		return cur != nil && cur.line == lineIdx && cur.col == start
	})
}
