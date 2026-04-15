package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// tableHit locates a substring match inside the results table. A single cell
// can contain multiple hits, and one row can contribute several across cells.
type tableHit struct {
	row    int
	cell   int
	col    int // byte offset in the formatted cell string
	length int // byte length of the match
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

// computeTableHits scans every cell in the result rows for case-insensitive
// occurrences of query. Rows are walked in display order so n/N cycles
// top-to-bottom, left-to-right across the full result set.
func computeTableHits(result *queryResult, query string) []tableHit {
	if result == nil || query == "" {
		return nil
	}
	needle := strings.ToLower(query)
	var hits []tableHit
	for r, row := range result.Rows {
		for c := range result.Columns {
			var cell any
			if c < len(row) {
				cell = row[c]
			}
			text := formatCell(cell)
			lower := strings.ToLower(text)
			start := 0
			for {
				idx := strings.Index(lower[start:], needle)
				if idx < 0 {
					break
				}
				hits = append(hits, tableHit{
					row:    r,
					cell:   c,
					col:    start + idx,
					length: len(needle),
				})
				start += idx + len(needle)
				if len(needle) == 0 {
					break
				}
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
	needle := strings.ToLower(query)
	var hits []jsonHit
	lines := strings.Split(jsonRaw, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		start := 0
		for {
			idx := strings.Index(lower[start:], needle)
			if idx < 0 {
				break
			}
			hits = append(hits, jsonHit{
				line:   i,
				col:    start + idx,
				length: len(needle),
			})
			start += idx + len(needle)
			if len(needle) == 0 {
				break
			}
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
	if query == "" || text == "" {
		return text
	}
	needle := strings.ToLower(query)
	lower := strings.ToLower(text)
	var b strings.Builder
	start := 0
	for {
		idx := strings.Index(lower[start:], needle)
		if idx < 0 {
			b.WriteString(text[start:])
			break
		}
		b.WriteString(text[start : start+idx])
		matchStart := start + idx
		matchEnd := matchStart + len(needle)
		span := text[matchStart:matchEnd]
		style := searchMatchStyle
		if cur != nil && cur.row == row && cur.cell == cell && cur.col == matchStart {
			style = searchCurrentStyle
		}
		b.WriteString(style.Render(span))
		start = matchEnd
		if len(needle) == 0 {
			break
		}
	}
	return b.String()
}

// highlightJSONLine decorates one plain-text JSON line with match highlights.
// When the line holds the current cursor hit, that span uses the current
// style. Used only on lines where at least one match exists; untouched lines
// keep their Chroma colouring in refreshJSON.
func highlightJSONLine(line, query string, cur *jsonHit, lineIdx int) string {
	if query == "" || line == "" {
		return line
	}
	needle := strings.ToLower(query)
	lower := strings.ToLower(line)
	var b strings.Builder
	start := 0
	for {
		idx := strings.Index(lower[start:], needle)
		if idx < 0 {
			b.WriteString(line[start:])
			break
		}
		b.WriteString(line[start : start+idx])
		matchStart := start + idx
		matchEnd := matchStart + len(needle)
		span := line[matchStart:matchEnd]
		style := searchMatchStyle
		if cur != nil && cur.line == lineIdx && cur.col == matchStart {
			style = searchCurrentStyle
		}
		b.WriteString(style.Render(span))
		start = matchEnd
		if len(needle) == 0 {
			break
		}
	}
	return b.String()
}
