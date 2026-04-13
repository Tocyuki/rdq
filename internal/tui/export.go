package tui

import (
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// writeCSV serializes the result as CSV. Numbers, bools, and strings are
// written in their natural form; NULL becomes the empty field, blobs become
// base64, and arrays use the same JSON-ish notation as the table view so
// existing readers can interpret them visually.
func (r *queryResult) writeCSV(w io.Writer) error {
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

// exportCSV writes the result to a timestamped file in the current working
// directory and returns its path. Filename collisions are resolved by adding
// a counter suffix so two presses within the same second do not overwrite.
func (r *queryResult) exportCSV() (string, error) {
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
	if err := r.writeCSV(f); err != nil {
		return "", err
	}
	return path, nil
}

// shortenPath replaces $HOME with ~ for compact display in the status line.
func shortenPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}
