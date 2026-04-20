package runner

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata/types"
)

// Result is a UI-friendly view of an RDS Data API ExecuteStatement response.
// Cell values are kept in their native Go types so JSON and CSV exports can
// preserve type information; table rendering converts them to strings on demand.
type Result struct {
	Columns []string
	Rows    [][]any

	// Updated is the number of rows affected for INSERT/UPDATE/DELETE.
	// -1 means "not applicable" (e.g. SELECT).
	Updated int64

	// cachedWidths memoizes ColumnWidths(Columns, Rows) since Rows is
	// immutable after ConvertResult.
	cachedWidths []int
}

// NullDisplay is the canonical NULL marker shown in the table and JSON view.
const NullDisplay = "NULL"

// ColumnWidthCap caps any single column at this many display cells before
// truncation kicks in, so a very wide value cannot push other columns off
// screen.
const ColumnWidthCap = 40

// Widths returns the rendered display width for each column, caching the
// result so a table view can scroll horizontally without re-walking every
// cell on each keystroke.
func (r *Result) Widths() []int {
	if r == nil {
		return nil
	}
	if r.cachedWidths == nil {
		r.cachedWidths = ColumnWidths(r.Columns, r.Rows)
	}
	return r.cachedWidths
}

// ConvertResult turns an ExecuteStatement output into a Result by walking the
// column metadata for headers and the records grid for cell values. Row length
// is normalized to len(Columns) so callers can rely on the invariant that
// every row has exactly one cell per column.
func ConvertResult(out *rdsdata.ExecuteStatementOutput) *Result {
	if out == nil {
		return &Result{}
	}
	res := &Result{Updated: out.NumberOfRecordsUpdated}

	for _, col := range out.ColumnMetadata {
		res.Columns = append(res.Columns, aws.ToString(col.Name))
	}

	for _, record := range out.Records {
		row := make([]any, len(res.Columns))
		for i := range row {
			if i < len(record) {
				row[i] = fieldValue(record[i])
			}
		}
		res.Rows = append(res.Rows, row)
	}
	return res
}

// fieldValue unwraps an RDS Data API tagged-union Field into the closest
// native Go value. Returning interface{} (any) means JSON marshaling preserves
// type information instead of stringifying everything.
func fieldValue(f types.Field) any {
	if f == nil {
		return nil
	}
	switch v := f.(type) {
	case *types.FieldMemberIsNull:
		return nil
	case *types.FieldMemberStringValue:
		return v.Value
	case *types.FieldMemberLongValue:
		return v.Value
	case *types.FieldMemberDoubleValue:
		return v.Value
	case *types.FieldMemberBooleanValue:
		return v.Value
	case *types.FieldMemberBlobValue:
		return v.Value
	case *types.FieldMemberArrayValue:
		return arrayValue(v.Value)
	}
	return nil
}

// arrayValue converts an RDS Data API ArrayValue (one of several typed array
// variants) into a generic []any slice.
func arrayValue(a types.ArrayValue) any {
	if a == nil {
		return []any{}
	}
	switch v := a.(type) {
	case *types.ArrayValueMemberStringValues:
		out := make([]any, len(v.Value))
		for i, s := range v.Value {
			out[i] = s
		}
		return out
	case *types.ArrayValueMemberLongValues:
		out := make([]any, len(v.Value))
		for i, n := range v.Value {
			out[i] = n
		}
		return out
	case *types.ArrayValueMemberDoubleValues:
		out := make([]any, len(v.Value))
		for i, n := range v.Value {
			out[i] = n
		}
		return out
	case *types.ArrayValueMemberBooleanValues:
		out := make([]any, len(v.Value))
		for i, b := range v.Value {
			out[i] = b
		}
		return out
	case *types.ArrayValueMemberArrayValues:
		out := make([]any, len(v.Value))
		for i, inner := range v.Value {
			out[i] = arrayValue(inner)
		}
		return out
	}
	return []any{}
}

// FormatCell renders a typed cell value as a display string for the table
// view. NULL becomes "NULL", blobs become "<blob N bytes>", and arrays use a
// JSON-ish notation produced by recursively formatting their elements.
func FormatCell(v any) string {
	if v == nil {
		return NullDisplay
	}
	switch val := v.(type) {
	case string:
		return val
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	case []byte:
		return fmt.Sprintf("<blob %d bytes>", len(val))
	case []any:
		return formatArray(val)
	}
	return fmt.Sprintf("%v", v)
}

func formatArray(arr []any) string {
	if len(arr) == 0 {
		return "[]"
	}
	parts := make([]string, len(arr))
	for i, v := range arr {
		switch val := v.(type) {
		case string:
			parts[i] = strconv.Quote(val)
		default:
			parts[i] = FormatCell(val)
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// ColumnWidths returns the rendered column widths needed to fit all headers
// and cell values, capped at ColumnWidthCap per column.
func ColumnWidths(columns []string, rows [][]any) []int {
	widths := make([]int, len(columns))
	for i, c := range columns {
		widths[i] = DisplayWidth(c)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				continue
			}
			w := DisplayWidth(FormatCell(cell))
			if w > widths[i] {
				widths[i] = w
			}
		}
	}
	for i := range widths {
		if widths[i] > ColumnWidthCap {
			widths[i] = ColumnWidthCap
		}
		if widths[i] < 1 {
			widths[i] = 1
		}
	}
	return widths
}

// DisplayWidth approximates the visible width of a cell. We deliberately use
// rune count rather than wcwidth because the input is already free of escape
// sequences and the table width only needs to be roughly accurate.
func DisplayWidth(s string) int {
	return len([]rune(s))
}

// Truncate shortens s to at most width display cells, appending an ellipsis
// when content was dropped.
func Truncate(s string, width int) string {
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

// RowJSON renders a single row as a pretty-printed JSON object with the
// original column order preserved. encoding/json sorts map keys alphabetically,
// which loses the SELECT order users expect, so we build the outer object by
// hand and only delegate to json.MarshalIndent for individual values (which
// keeps native typing for numbers, bools, blobs, and nulls).
func (r *Result) RowJSON(idx int) (string, error) {
	if r == nil || idx < 0 || idx >= len(r.Rows) {
		return "", fmt.Errorf("row index %d out of range", idx)
	}
	row := r.Rows[idx]

	var b strings.Builder
	b.WriteString("{\n")
	for i, col := range r.Columns {
		var val any
		if i < len(row) {
			val = row[i]
		}
		valJSON, err := json.MarshalIndent(val, "  ", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal column %q: %w", col, err)
		}
		keyJSON, _ := json.Marshal(col)
		b.WriteString("  ")
		b.Write(keyJSON)
		b.WriteString(": ")
		b.Write(valJSON)
		if i < len(r.Columns)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}")
	return b.String(), nil
}

// ToJSON renders the result as a pretty-printed JSON array of row objects.
// Because Rows is [][]any with native Go types, encoding/json preserves
// number/bool/null/blob(base64) without lossy stringification.
func (r *Result) ToJSON() string {
	if r == nil {
		return "null"
	}
	rows := make([]map[string]any, 0, len(r.Rows))
	for _, row := range r.Rows {
		obj := make(map[string]any, len(r.Columns))
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

// WriteCSV serializes the result as CSV. Numbers, bools, and strings are
// written in their natural form; NULL becomes the empty field, blobs become
// base64, and arrays use the same JSON-ish notation as the table view so
// existing readers can interpret them visually.
func (r *Result) WriteCSV(w io.Writer) error {
	if r == nil {
		return nil
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(r.Columns); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, row := range r.Rows {
		record := make([]string, len(r.Columns))
		for i := range record {
			if i < len(row) {
				record[i] = formatCSVCell(row[i])
			}
		}
		if err := cw.Write(record); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// formatCSVCell renders a typed cell for CSV output. NULL is the empty string
// (the conventional CSV NULL representation), blobs are base64, and arrays
// reuse formatArray.
func formatCSVCell(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	case []byte:
		return base64.StdEncoding.EncodeToString(val)
	case []any:
		return formatArray(val)
	}
	return fmt.Sprintf("%v", v)
}

// ExportCSV writes the result to a timestamped file in the current working
// directory and returns its path. Filename collisions are resolved by adding
// a counter suffix so two presses within the same second do not overwrite.
func (r *Result) ExportCSV() (string, error) {
	if r == nil || len(r.Columns) == 0 {
		return "", fmt.Errorf("no result to export")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	base := "rdq-" + time.Now().Format("20060102-150405")
	path := filepath.Join(cwd, base+".csv")
	for i := 1; ; i++ {
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			break
		}
		path = filepath.Join(cwd, base+"-"+strconv.Itoa(i)+".csv")
		if i > 100 {
			return "", fmt.Errorf("could not find an unused export filename")
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create csv file: %w", err)
	}
	defer f.Close()
	if err := r.WriteCSV(f); err != nil {
		return "", err
	}
	return path, nil
}
