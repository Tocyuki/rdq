package runner

import (
	"fmt"
	"io"
	"strings"
)

// RenderTable writes Result as a psql-style bordered ASCII table.
//
// Numeric columns (int64/float64) are right-aligned so digits line up by
// magnitude; everything else is left-aligned. Cells wider than ColumnWidthCap
// are truncated via Truncate so a single long value cannot push other columns
// off screen. SELECT-shaped results (Updated < 0) append a "(N rows)" footer;
// write-shaped results leave the footer to the caller so it can be routed to
// stderr and keep stdout free of non-data noise.
//
// r == nil or a result with no columns renders nothing.
func RenderTable(w io.Writer, r *Result) error {
	if r == nil || len(r.Columns) == 0 {
		return nil
	}
	widths := r.Widths()
	alignRight := detectNumericColumns(r.Columns, r.Rows)

	sep := buildSeparatorRow(widths)

	if _, err := fmt.Fprintln(w, sep); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, buildDataRow(r.Columns, widths, nil)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, sep); err != nil {
		return err
	}

	for _, row := range r.Rows {
		cells := make([]string, len(r.Columns))
		for i := range r.Columns {
			if i < len(row) {
				cells[i] = FormatCell(row[i])
			} else {
				cells[i] = NullDisplay
			}
		}
		if _, err := fmt.Fprintln(w, buildDataRow(cells, widths, alignRight)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, sep); err != nil {
		return err
	}

	if r.Updated < 0 {
		if _, err := fmt.Fprintf(w, "(%d %s)\n", len(r.Rows), rowsWord(len(r.Rows))); err != nil {
			return err
		}
	}
	return nil
}

// buildSeparatorRow returns "+----+----+" style row that frames the table.
// Each segment is widths[i]+2 dashes to account for the single-space padding
// on either side of the cell content.
func buildSeparatorRow(widths []int) string {
	var b strings.Builder
	b.WriteByte('+')
	for _, w := range widths {
		b.WriteString(strings.Repeat("-", w+2))
		b.WriteByte('+')
	}
	return b.String()
}

// buildDataRow returns "| val | val |" style row. Cells longer than the
// column width are truncated via Truncate. alignRight[i] == true pads on the
// left so numeric columns line up by magnitude; a nil alignRight (header row)
// left-aligns every cell.
func buildDataRow(cells []string, widths []int, alignRight []bool) string {
	var b strings.Builder
	b.WriteByte('|')
	for i, cell := range cells {
		w := widths[i]
		trimmed := cell
		if DisplayWidth(trimmed) > w {
			trimmed = Truncate(trimmed, w)
		}
		pad := w - DisplayWidth(trimmed)
		b.WriteByte(' ')
		right := alignRight != nil && i < len(alignRight) && alignRight[i]
		if right {
			b.WriteString(strings.Repeat(" ", pad))
			b.WriteString(trimmed)
		} else {
			b.WriteString(trimmed)
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteByte(' ')
		b.WriteByte('|')
	}
	return b.String()
}

// detectNumericColumns returns a bool per column indicating whether the
// column's values are purely numeric (int64 or float64). NULLs are ignored
// so "id | 1 | NULL | 2" still counts as numeric. Empty columns default to
// left-aligned (false).
func detectNumericColumns(columns []string, rows [][]any) []bool {
	out := make([]bool, len(columns))
	seen := make([]bool, len(columns))
	for i := range out {
		out[i] = true
	}
	for _, row := range rows {
		for i := range columns {
			if i >= len(row) || row[i] == nil {
				continue
			}
			seen[i] = true
			switch row[i].(type) {
			case int64, float64:
				// keep true
			default:
				out[i] = false
			}
		}
	}
	for i := range out {
		if !seen[i] {
			out[i] = false
		}
	}
	return out
}

// rowsWord returns "row" for exactly one record and "rows" otherwise so the
// footer reads naturally ("(1 row)" / "(2 rows)"), matching psql.
func rowsWord(n int) string {
	if n == 1 {
		return "row"
	}
	return "rows"
}
