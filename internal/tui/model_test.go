package tui

import (
	"strconv"
	"testing"
)

// makeResult builds a synthetic queryResult with the given column and row
// counts so refreshTable's table-update path can be exercised without an
// actual RDS Data API round trip.
func makeResult(cols, rows int) *queryResult {
	r := &queryResult{
		Columns: make([]string, cols),
		Rows:    make([][]any, rows),
	}
	for c := 0; c < cols; c++ {
		r.Columns[c] = "col" + strconv.Itoa(c)
	}
	for i := 0; i < rows; i++ {
		row := make([]any, cols)
		for c := 0; c < cols; c++ {
			row[c] = "v" + strconv.Itoa(i) + "_" + strconv.Itoa(c)
		}
		r.Rows[i] = row
	}
	return r
}

// TestRefreshTableHandlesShrinkingColumnCount guards against the original
// panic where bubbles/table.SetColumns synchronously rendered stale rows
// against the new (narrower) column slice and indexed out of range.
func TestRefreshTableHandlesShrinkingColumnCount(t *testing.T) {
	m := newModel(nil, target{}, nil)

	m.result = makeResult(10, 5)
	m.refreshTable()

	m.result = makeResult(9, 3)
	m.refreshTable()

	m.result = makeResult(1, 0)
	m.refreshTable()

	m.result = nil
	m.refreshTable()
}

func TestRefreshTableHandlesGrowingColumnCount(t *testing.T) {
	m := newModel(nil, target{}, nil)

	m.result = makeResult(2, 4)
	m.refreshTable()

	m.result = makeResult(15, 2)
	m.refreshTable()
}
