package runner

import "testing"

func TestIsReadOnlySQL(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want bool
	}{
		{"simple select", "SELECT 1", true},
		{"select lowercase", "select * from users", true},
		{"leading whitespace", "\n  \t SELECT 1", true},
		{"leading nbsp", "\u00a0SELECT 1", true},
		{"leading ideographic space", "\u3000SELECT 1", true},
		{"leading BOM", "\ufeffSELECT 1", true},
		{"line comment", "-- a comment\nSELECT 1", true},
		{"multiple line comments", "-- one\n-- two\nSELECT 1", true},
		{"block comment", "/* pre */ SELECT 1", true},
		{"nested-looking block comment", "/* nested /* inner */ SELECT 1", true},
		{"with CTE", "WITH c AS (SELECT 1) SELECT * FROM c", true},
		{"with recursive CTE", "WITH RECURSIVE c(n) AS (SELECT 1) SELECT * FROM c", true},
		{"with quoted CTE name", `WITH "recent_users" AS (SELECT 1) SELECT * FROM "recent_users"`, true},
		{"show", "SHOW TABLES", true},
		{"explain", "EXPLAIN SELECT 1", true},
		{"describe", "DESCRIBE users", true},
		{"desc", "DESC users", true},
		{"table shorthand", "TABLE users", true},
		{"values", "VALUES (1, 2), (3, 4)", true},

		{"insert", "INSERT INTO t VALUES (1)", false},
		{"update", "UPDATE t SET x = 1", false},
		{"delete", "DELETE FROM t", false},
		{"alter", "ALTER TABLE t ADD COLUMN y INT", false},
		{"drop", "DROP TABLE t", false},
		{"create", "CREATE TABLE t (id INT)", false},
		{"truncate", "TRUNCATE TABLE t", false},
		{"grant", "GRANT SELECT ON t TO r", false},
		{"revoke", "REVOKE SELECT ON t FROM r", false},
		{"use", "USE mydb", false},
		{"set", "SET autocommit = 0", false},
		{"call", "CALL my_proc()", false},
		{"merge", "MERGE INTO t USING ...", false},
		{"with delete CTE", "WITH gone AS (DELETE FROM t RETURNING id) SELECT * FROM gone", false},
		{"with update CTE", "WITH changed AS (UPDATE t SET x = 1 RETURNING id) SELECT * FROM changed", false},
		{"with insert CTE", "WITH added AS (INSERT INTO t(id) VALUES (1) RETURNING id) SELECT * FROM added", false},
		{"with malformed cte", "WITH c AS SELECT 1", false},
		{"empty", "", false},
		{"whitespace only", "   \t\n ", false},
		{"only comments", "-- nothing here", false},
		{"unclosed block comment", "/* never closed", false},
		{"non-alpha start", "123 SELECT", false},
		{"semicolon-led", ";SELECT 1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsReadOnlySQL(tc.sql)
			if got != tc.want {
				t.Errorf("IsReadOnlySQL(%q) = %v, want %v", tc.sql, got, tc.want)
			}
		})
	}
}
