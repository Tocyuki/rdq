package tui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata/types"
)

// queryResult is the TUI-friendly view of an RDS Data API ExecuteStatement
// response. All cell values are pre-stringified for table rendering.
type queryResult struct {
	Columns []string
	Rows    [][]string

	// Updated is the number of rows affected for INSERT/UPDATE/DELETE.
	// -1 means "not applicable" (e.g. SELECT).
	Updated int64
}

// columnWidthCap caps any single column at this many display cells before
// truncation kicks in, so a very wide value cannot push other columns off
// screen.
const columnWidthCap = 40

// nullDisplay is the canonical NULL marker shown in the table.
const nullDisplay = "NULL"

// convertResult turns an ExecuteStatement output into a queryResult by walking
// the column metadata for headers and the records grid for cell values.
func convertResult(out *rdsdata.ExecuteStatementOutput) *queryResult {
	if out == nil {
		return &queryResult{}
	}

	res := &queryResult{Updated: out.NumberOfRecordsUpdated}

	for _, col := range out.ColumnMetadata {
		res.Columns = append(res.Columns, aws.ToString(col.Name))
	}

	// Normalize row length to len(Columns) so downstream code (notably
	// refreshTable and bubbles/table) can rely on the invariant that every
	// row has exactly one cell per column. Servers and proxies sometimes
	// return mismatched record/metadata lengths, and tolerating that here
	// keeps the rendering layer simple.
	for _, record := range out.Records {
		row := make([]string, len(res.Columns))
		for i := range row {
			if i < len(record) {
				row[i] = fieldToString(record[i])
			} else {
				row[i] = nullDisplay
			}
		}
		res.Rows = append(res.Rows, row)
	}
	return res
}

// fieldToString renders a single RDS Data API Field value as a display string.
// The Data API uses a tagged-union pattern (FieldMember*) so a type switch is
// the canonical way to dispatch.
func fieldToString(f types.Field) string {
	if f == nil {
		return nullDisplay
	}
	switch v := f.(type) {
	case *types.FieldMemberIsNull:
		return nullDisplay
	case *types.FieldMemberStringValue:
		return v.Value
	case *types.FieldMemberLongValue:
		return strconv.FormatInt(v.Value, 10)
	case *types.FieldMemberDoubleValue:
		return strconv.FormatFloat(v.Value, 'f', -1, 64)
	case *types.FieldMemberBooleanValue:
		return strconv.FormatBool(v.Value)
	case *types.FieldMemberBlobValue:
		return fmt.Sprintf("<blob %d bytes>", len(v.Value))
	case *types.FieldMemberArrayValue:
		return arrayToString(v.Value)
	}
	return "?"
}

// arrayToString renders an ArrayValue (one of several typed array variants)
// as a JSON-ish list. Nested arrays are not expected from the Data API in
// practice but we handle them recursively for safety.
func arrayToString(a types.ArrayValue) string {
	if a == nil {
		return "[]"
	}
	switch v := a.(type) {
	case *types.ArrayValueMemberStringValues:
		quoted := make([]string, len(v.Value))
		for i, s := range v.Value {
			quoted[i] = strconv.Quote(s)
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	case *types.ArrayValueMemberLongValues:
		parts := make([]string, len(v.Value))
		for i, n := range v.Value {
			parts[i] = strconv.FormatInt(n, 10)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *types.ArrayValueMemberDoubleValues:
		parts := make([]string, len(v.Value))
		for i, n := range v.Value {
			parts[i] = strconv.FormatFloat(n, 'f', -1, 64)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *types.ArrayValueMemberBooleanValues:
		parts := make([]string, len(v.Value))
		for i, b := range v.Value {
			parts[i] = strconv.FormatBool(b)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *types.ArrayValueMemberArrayValues:
		parts := make([]string, len(v.Value))
		for i, inner := range v.Value {
			parts[i] = arrayToString(inner)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return "[]"
}

// columnWidths returns the rendered column widths needed to fit all headers
// and rows, capped at columnWidthCap per column.
func columnWidths(columns []string, rows [][]string) []int {
	widths := make([]int, len(columns))
	for i, c := range columns {
		widths[i] = displayWidth(c)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				continue
			}
			w := displayWidth(cell)
			if w > widths[i] {
				widths[i] = w
			}
		}
	}
	for i := range widths {
		if widths[i] > columnWidthCap {
			widths[i] = columnWidthCap
		}
		if widths[i] < 1 {
			widths[i] = 1
		}
	}
	return widths
}

// displayWidth approximates the visible width of a cell. We deliberately use
// rune count rather than wcwidth because the input is already free of escape
// sequences and the table width only needs to be roughly accurate.
func displayWidth(s string) int {
	return len([]rune(s))
}

// truncate shortens s to at most width display cells, appending an ellipsis
// when content was dropped.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

// toJSON renders the result as a pretty-printed JSON array of row objects so
// it can be inspected when the table view is too lossy (e.g. blobs or
// many-column results).
func (r *queryResult) toJSON() string {
	if r == nil {
		return "null"
	}
	rows := make([]map[string]string, 0, len(r.Rows))
	for _, row := range r.Rows {
		obj := make(map[string]string, len(r.Columns))
		for i, col := range r.Columns {
			if i < len(row) {
				obj[col] = row[i]
			}
		}
		rows = append(rows, obj)
	}
	out, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return fmt.Sprintf("json marshal error: %v", err)
	}
	return string(out)
}

// blobBase64 is exposed for tests and reserved for future use; it produces the
// canonical base64 encoding of a blob field, mirroring how the Data API
// returns blobs over the wire.
func blobBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
