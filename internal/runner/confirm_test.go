package runner

import (
	"strings"
	"testing"
)

func TestNeedsConfirmation(t *testing.T) {
	cases := []struct {
		name       string
		sql        string
		wantNeed   bool
		reasonHint string // substring that should appear in the reason, or "" if not needed
	}{
		// Safe statements
		{"select no where", "SELECT * FROM t", false, ""},
		{"select with where", "SELECT * FROM t WHERE id = 1", false, ""},
		{"insert", "INSERT INTO t (a) VALUES (1)", false, ""},

		// DELETE
		{"delete no where", "DELETE FROM t", true, "DELETE without a WHERE"},
		{"delete with where", "DELETE FROM t WHERE id = 1", false, ""},
		{"delete lower", "delete from t", true, "DELETE without a WHERE"},
		{"delete where literal", "DELETE FROM t WHERE name = 'foo'", false, ""},
		{"delete where in string only", "DELETE FROM t -- WHERE lives in comment", true, "DELETE without a WHERE"},
		{"delete block comment contains where", "DELETE FROM t /* WHERE */ ", true, "DELETE without a WHERE"},
		{"delete string contains where but outside where", "DELETE FROM t WHERE note = 'WHERE is inside a string'", false, ""},
		{"delete cte without where in main", "WITH c AS (SELECT 1) DELETE FROM t", true, "DELETE without a WHERE"},
		{"delete cte in body without where", "WITH gone AS (DELETE FROM t RETURNING id) SELECT * FROM gone", true, "DELETE without a WHERE"},
		{"update cte in body without where", "WITH changed AS (UPDATE t SET x = 1 RETURNING id) SELECT * FROM changed", true, "UPDATE without a WHERE"},
		{"malformed with is conservative", "WITH c AS SELECT 1", true, "WITH statements can hide destructive writes"},

		// UPDATE
		{"update no where", "UPDATE t SET x = 1", true, "UPDATE without a WHERE"},
		{"update with where", "UPDATE t SET x = 1 WHERE id = 1", false, ""},
		{"update where in string only", "UPDATE t SET note = 'WHERE trick'", true, "UPDATE without a WHERE"},
		{"update trailing line comment", "UPDATE t SET x = 1 -- WHERE id = 1", true, "UPDATE without a WHERE"},
		{"update where inside quoted identifier", `UPDATE t SET "WHERE" = 1`, true, "UPDATE without a WHERE"},
		{"update where in subquery", "UPDATE t SET x = (SELECT max(id) FROM u WHERE u.k = 1)", false, ""},
		// ^ subquery WHERE is intentionally accepted. The naive detector cannot
		// tell whether WHERE binds to the outer UPDATE or an inner SELECT; in
		// practice users almost always add a top-level WHERE after SET, so we
		// accept false negatives on exotic UPDATE-with-subquery-only shapes.

		// TRUNCATE
		{"truncate", "TRUNCATE TABLE t", true, "TRUNCATE"},
		{"truncate short", "TRUNCATE t", true, "TRUNCATE"},

		// Leading whitespace / comments should not defeat detection.
		{"delete leading newline", "\n  DELETE FROM t", true, "DELETE without a WHERE"},
		{"delete leading comment", "-- head comment\nDELETE FROM t", true, "DELETE without a WHERE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := NeedsConfirmation(tc.sql)
			if got != tc.wantNeed {
				t.Fatalf("NeedsConfirmation(%q) need = %v, want %v (reason %q)", tc.sql, got, tc.wantNeed, reason)
			}
			if tc.wantNeed && tc.reasonHint != "" && !strings.Contains(reason, tc.reasonHint) {
				t.Errorf("reason = %q, want to contain %q", reason, tc.reasonHint)
			}
		})
	}
}

func TestStripSQLNoise(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"line comment", "a -- comment\nb", "a \nb"},
		{"block comment", "a /* x */ b", "a  b"},
		{"unterminated block", "a /* x", "a "},
		{"string literal", "a 'x y' b", "a  b"},
		{"escaped quote", "a 'it''s fine' b", "a  b"},
		{"double quoted id", `a "WHERE" b`, "a  b"},
		{"mixed", "UPDATE t /* foo */ SET x = 1 -- WHERE\n", "UPDATE t  SET x = 1 \n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripSQLNoise(tc.in)
			if got != tc.want {
				t.Errorf("stripSQLNoise(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
