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
		// PostgreSQL nests block comments, so there this comment never
		// closes, while MySQL reads `SELECT 1`. Ambiguous → reject.
		{"nested-looking block comment", "/* nested /* inner */ SELECT 1", false},
		{"properly nested block comment", "/* outer /* inner */ */ SELECT 1", false}, // MySQL sees `*/ SELECT 1`
		{"pg nested comment hides into", "SELECT 1 /* /* */ ' */ INTO t -- '", false},
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

		// Multiple statements: every statement must be read-only.
		{"trailing semicolon", "SELECT 1;", true},
		{"trailing semicolon and comment", "SELECT 1; -- done", true},
		{"two selects", "SELECT 1; SELECT 2", true},
		{"select then delete", "SELECT 1; DELETE FROM t", false},
		{"select then drop", "select 1;drop table t", false},
		{"semicolon inside string", "SELECT ';DELETE FROM t'", true},
		{"semicolon inside comment", "SELECT 1 /* ; DELETE FROM t */", true},
		// MySQL has no dollar quotes, so there this is two statements.
		{"semicolon inside dollar quote", "SELECT $$;DELETE FROM t$$", false},
		{"dollar quote without semicolon", "SELECT $$fine$$", true},
		// In MySQL the quote inside opens a string that never closes.
		{"dollar quote with apostrophe", "SELECT $$it's$$", false},
		{"mysql double dash before DEL", "SELECT 1 --\x7f '\nINTO OUTFILE 'x' -- '", false},
		{"mysql ansi quotes", `SELECT "a\" INTO OUTFILE 'x' -- "`, false},
		{"mysql no backslash escapes", `SELECT 'a\' INTO OUTFILE 'x' -- '`, false},
		{"unterminated string", "SELECT 'abc", false},
		// PostgreSQL with standard_conforming_strings=off: `'a\''` is the
		// complete string "a'", so the INTO after it is live code.
		{"pg non-standard strings", `SELECT 'a\'' INTO t -- '`, false},
		{"pg non-standard strings hides semicolon", `SELECT 'a\''; DELETE FROM t; -- '`, false},
		// Only the non-standard-strings reading sees INTO here: standard
		// PostgreSQL and NO_BACKSLASH_ESCAPES keep it inside the string,
		// and the other MySQL modes treat `# ...` as a comment.
		{"pg non-standard strings only", `SELECT 'a\'' # INTO t -- '`, false},

		// Dialect differentials: what one engine lexes as a comment or
		// string must not hide code the other engine executes.
		{"mysql executable comment", "SELECT 1 /*! INTO OUTFILE '/tmp/x' */", false},
		{"mariadb executable comment", "SELECT 1 /*M!100100 INTO OUTFILE '/tmp/x' */", false},
		{"mysql versioned executable comment", "SELECT 1 /*!50100 INTO @v */", false},
		{"optimizer hint is a comment", "SELECT /*+ MAX_EXECUTION_TIME(1000) */ 1", true},
		{"mysql backslash escape", `SELECT 'a\'' INTO OUTFILE '/tmp/x' -- '`, false},
		{"mysql backslash in double quotes", `SELECT "a\"" INTO OUTFILE '/tmp/x' -- "`, false},
		{"postgres E string", `SELECT E'a\'' INTO t -- '`, false},
		{"mysql double dash without space", "SELECT 1 --1 INTO OUTFILE '/tmp/x'", false},
		{"mysql hash comment hides quote", "SELECT 1 # '\nINTO OUTFILE '/tmp/x' -- '", false},
		{"backslash in literal is fine", `SELECT * FROM t WHERE name LIKE 'a\_b'`, true},
		{"windows path literal", `SELECT 'C:\temp\x'`, true},
		{"hash comment", "# note\nSELECT 1", false}, // PostgreSQL has no # comments

		// EXPLAIN is only as read-only as the statement it explains.
		{"explain analyze select", "EXPLAIN ANALYZE SELECT 1", true},
		{"explain analyze delete", "EXPLAIN ANALYZE DELETE FROM t", false},
		{"explain analyse verbose update", "explain analyse verbose UPDATE t SET x = 1", false},
		{"explain options delete", "EXPLAIN (ANALYZE, BUFFERS) DELETE FROM t", false},
		{"explain options select", "EXPLAIN (ANALYZE, FORMAT JSON) SELECT 1", true},
		{"explain mysql format", "EXPLAIN FORMAT=JSON SELECT 1", true},
		{"explain mysql analyze insert", "EXPLAIN ANALYZE INSERT INTO t VALUES (1)", false},
		{"explain bare", "EXPLAIN", false},

		// SELECT ... INTO writes.
		{"select into table", "SELECT * INTO new_t FROM t", false},
		{"select into outfile", "SELECT * FROM t INTO OUTFILE '/tmp/x'", false},
		{"select into after comment", "SELECT 1/**/INTO t", false},
		{"select into in string", "SELECT 'INTO' AS word", true},
		{"cte tail select into", "WITH c AS (SELECT 1) SELECT * INTO t FROM c", false},
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
