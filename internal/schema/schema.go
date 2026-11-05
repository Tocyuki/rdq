// Package schema fetches and caches database table/column metadata from
// information_schema so the TUI can include schema context in AI prompts.
//
// The cache lives at ~/.rdq/schema/<sha256(cluster:database)[:16]>.json
// (overridable via RDQ_SCHEMA_DIR). There is no automatic TTL — users can
// manually delete a cache file to force a re-fetch.
package schema

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Tocyuki/rdq/internal/fsutil"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata/types"
)

// Column is one row from information_schema.columns flattened into a struct.
type Column struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	// PrimaryKey is the column's 1-based position in its table's primary
	// key, or 0 when it is not part of one.
	PrimaryKey int `json:"primary_key,omitempty"`
}

// Snapshot captures the full set of columns for one (cluster, database) pair
// at a point in time.
type Snapshot struct {
	// Version is the cache format version (see snapshotVersion).
	Version   int       `json:"version,omitempty"`
	Cluster   string    `json:"cluster"`
	Database  string    `json:"database"`
	FetchedAt time.Time `json:"fetched_at"`
	Columns   []Column  `json:"columns"`
	// PrimaryKeysKnown reports whether Column.PrimaryKey was populated.
	// It is false for caches written by older rdq versions and when the
	// primary-key lookup failed, so consumers can tell "no primary key"
	// apart from "unknown".
	PrimaryKeysKnown bool `json:"primary_keys_known,omitempty"`
}

// fetchQueryFiltered joins information_schema.schemata to drop schemas
// owned by Aurora-managed system roles (`rdsadmin`) and the PostgreSQL
// built-in pseudo-owner of catalog schemas (`pg_database_owner`). These
// two consistently surface `information_schema` and `pg_catalog` as
// noise in the TUI / GUI sidebars and in AI schema prompts — hiding
// them upstream keeps the display focused on application schemas.
//
// `information_schema.schemata.schema_owner` is a PostgreSQL-specific
// column (the SQL standard defines the table but not the column), so on
// Aurora MySQL this query fails with "Unknown column" and the caller
// transparently falls back to fetchQuery.
const fetchQueryFiltered = `SELECT c.TABLE_SCHEMA, c.TABLE_NAME, c.COLUMN_NAME, c.DATA_TYPE
FROM information_schema.COLUMNS c
JOIN information_schema.SCHEMATA s ON s.SCHEMA_NAME = c.TABLE_SCHEMA
WHERE s.SCHEMA_OWNER NOT IN ('rdsadmin', 'pg_database_owner')
ORDER BY c.TABLE_SCHEMA, c.TABLE_NAME, c.ORDINAL_POSITION`

// fetchQuery is the fallback used when fetchQueryFiltered is rejected
// (most commonly on Aurora MySQL, which has no schema_owner column).
// Reserved words are uppercased so MySQL is happy; PostgreSQL is
// case-insensitive for unquoted identifiers so the same query parses
// cleanly there too.
const fetchQuery = `SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME, DATA_TYPE
FROM information_schema.COLUMNS
ORDER BY TABLE_SCHEMA, TABLE_NAME, ORDINAL_POSITION`

// primaryKeyQuery lists primary-key columns in key order. TABLE_CONSTRAINTS
// and KEY_COLUMN_USAGE are standard information_schema views available on
// both Aurora PostgreSQL and Aurora MySQL.
const primaryKeyQuery = `SELECT kcu.TABLE_SCHEMA, kcu.TABLE_NAME, kcu.COLUMN_NAME, kcu.ORDINAL_POSITION
FROM information_schema.TABLE_CONSTRAINTS tc
JOIN information_schema.KEY_COLUMN_USAGE kcu
  ON kcu.CONSTRAINT_SCHEMA = tc.CONSTRAINT_SCHEMA
 AND kcu.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
 AND kcu.TABLE_SCHEMA = tc.TABLE_SCHEMA
 AND kcu.TABLE_NAME = tc.TABLE_NAME
WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
ORDER BY kcu.TABLE_SCHEMA, kcu.TABLE_NAME, kcu.ORDINAL_POSITION`

// FetchTimeout is the overall budget for one Fetch (all pages plus the
// primary-key lookup). Shared by the TUI and the GUI server.
const FetchTimeout = 2 * time.Minute

// snapshotVersion is bumped whenever Snapshot gains data that older caches
// lack. LoadCache treats an older cache as a miss so it is refetched once
// automatically. Version 2 added primary-key information.
const snapshotVersion = 2

// fetchPageSize bounds the rows per ExecuteStatement call. The Data API
// rejects responses larger than 1 MB, which a single unpaged
// information_schema query exceeds on large databases; 2000 short rows
// stay far below that.
const fetchPageSize = 2000

// execFunc runs one SQL statement; it is the seam that lets tests drive
// the paging logic without a Data API client.
type execFunc func(ctx context.Context, sql string) (*rdsdata.ExecuteStatementOutput, error)

// Fetch executes the introspection query against the given cluster and
// database and returns a Snapshot. Caller is expected to wrap network errors
// with retry semantics if needed; this function returns them unwrapped so the
// TUI can decide whether to fall back to an empty schema.
//
// The owner-filtered query is attempted first so PostgreSQL users never
// see `information_schema` / `pg_catalog` noise in the sidebar. On the
// first error (typically Aurora MySQL, which lacks schema_owner) it
// retries with the simple query; only if the retry also fails does the
// error propagate. Results are fetched in pages (see fetchPageSize), and
// primary-key membership is looked up on a best-effort basis.
func Fetch(ctx context.Context, client *rdsdata.Client, cluster, secret, database string) (*Snapshot, error) {
	exec := func(ctx context.Context, sql string) (*rdsdata.ExecuteStatementOutput, error) {
		return executeFetch(ctx, client, cluster, secret, database, sql)
	}
	return fetchWith(ctx, exec, cluster, database)
}

func fetchWith(ctx context.Context, exec execFunc, cluster, database string) (*Snapshot, error) {
	records, err := fetchAllPages(ctx, exec, fetchQueryFiltered)
	if err != nil {
		records, err = fetchAllPages(ctx, exec, fetchQuery)
		if err != nil {
			return nil, fmt.Errorf("fetch information_schema: %w", err)
		}
	}

	snapshot := &Snapshot{
		Version:   snapshotVersion,
		Cluster:   cluster,
		Database:  database,
		FetchedAt: time.Now().UTC(),
	}
	for _, record := range records {
		if len(record) < 4 {
			continue
		}
		col := Column{
			Schema: stringField(record[0]),
			Table:  stringField(record[1]),
			Name:   stringField(record[2]),
			Type:   stringField(record[3]),
		}
		if col.Table == "" || col.Name == "" {
			continue
		}
		snapshot.Columns = append(snapshot.Columns, col)
	}

	// Primary keys are an enhancement (stable preview ordering), so a
	// failure here leaves PrimaryKeysKnown=false instead of failing Fetch.
	if pkRecords, err := fetchAllPages(ctx, exec, primaryKeyQuery); err == nil {
		applyPrimaryKeys(snapshot, pkRecords)
		snapshot.PrimaryKeysKnown = true
	}
	return snapshot, nil
}

// fetchAllPages runs base (which must end in a deterministic ORDER BY)
// repeatedly with LIMIT/OFFSET until a short page signals the end.
func fetchAllPages(ctx context.Context, exec execFunc, base string) ([][]types.Field, error) {
	var all [][]types.Field
	for offset := 0; ; offset += fetchPageSize {
		out, err := exec(ctx, fmt.Sprintf("%s\nLIMIT %d OFFSET %d", base, fetchPageSize, offset))
		if err != nil {
			return nil, err
		}
		all = append(all, out.Records...)
		if len(out.Records) != fetchPageSize {
			return all, nil
		}
	}
}

func applyPrimaryKeys(s *Snapshot, records [][]types.Field) {
	type colKey struct{ schema, table, name string }
	pos := make(map[colKey]int, len(records))
	for _, r := range records {
		if len(r) < 4 {
			continue
		}
		if n := intField(r[3]); n > 0 {
			pos[colKey{stringField(r[0]), stringField(r[1]), stringField(r[2])}] = n
		}
	}
	for i := range s.Columns {
		c := &s.Columns[i]
		c.PrimaryKey = pos[colKey{c.Schema, c.Table, c.Name}]
	}
}

// executeFetch is the thin Data API wrapper shared by Fetch's primary
// and fallback paths. Kept un-exported; production callers use Fetch.
func executeFetch(ctx context.Context, client *rdsdata.Client, cluster, secret, database, sql string) (*rdsdata.ExecuteStatementOutput, error) {
	return client.ExecuteStatement(ctx, &rdsdata.ExecuteStatementInput{
		ResourceArn:           aws.String(cluster),
		SecretArn:             aws.String(secret),
		Database:              aws.String(database),
		Sql:                   aws.String(sql),
		IncludeResultMetadata: true,
	})
}

// stringField extracts a string value from an RDS Data API Field, returning
// the empty string for any non-string variant.
func stringField(f types.Field) string {
	if v, ok := f.(*types.FieldMemberStringValue); ok {
		return v.Value
	}
	return ""
}

// intField extracts an integer from a Data API Field. information_schema
// ordinal columns come back as longs on MySQL and may come back as strings
// for PostgreSQL's cardinal_number domain.
func intField(f types.Field) int {
	switch v := f.(type) {
	case *types.FieldMemberLongValue:
		return int(v.Value)
	case *types.FieldMemberStringValue:
		n, _ := strconv.Atoi(strings.TrimSpace(v.Value))
		return n
	}
	return 0
}

// LoadCache reads a previously-saved Snapshot from disk. A missing file is
// not an error; callers can interpret nil as "no cache yet".
func LoadCache(cluster, database string) (*Snapshot, error) {
	path, err := cachePath(cluster, database)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read schema cache %s: %w", path, err)
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse schema cache %s: %w", path, err)
	}
	if s.Version < snapshotVersion {
		// Written by an older rdq: missing data (e.g. primary keys) would
		// silently degrade features, so report a miss and let the caller
		// refetch. Keyed on the version rather than PrimaryKeysKnown so a
		// current cache whose PK lookup legitimately failed is still used.
		return nil, nil
	}
	return &s, nil
}

// SaveCache writes a snapshot to disk atomically (tempfile + rename) so a
// concurrent reader never sees a half-written file.
func SaveCache(s *Snapshot) error {
	if s == nil {
		return errors.New("nil snapshot")
	}
	path, err := cachePath(s.Cluster, s.Database)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	return fsutil.WriteFileAtomic(path, data, 0o600)
}

// cachePath returns the on-disk path for a (cluster, database) snapshot. The
// directory is RDQ_SCHEMA_DIR if set, otherwise ~/.rdq/schema/.
func cachePath(cluster, database string) (string, error) {
	dir := os.Getenv("RDQ_SCHEMA_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		dir = filepath.Join(home, ".rdq", "schema")
	}
	hash := sha256.Sum256([]byte(cluster + ":" + database))
	name := hex.EncodeToString(hash[:])[:16] + ".json"
	return filepath.Join(dir, name), nil
}

// ToPrompt formats the snapshot as a CREATE TABLE-style listing suitable for
// embedding in an LLM system prompt. The output groups columns by table and
// lists "name type" for each. Schemas other than "public" or the database
// itself are prefixed.
func (s *Snapshot) ToPrompt() string {
	if s == nil || len(s.Columns) == 0 {
		return "(no schema available)"
	}

	type tableKey struct{ schema, table string }
	groups := map[tableKey][]Column{}
	keys := make([]tableKey, 0)
	for _, c := range s.Columns {
		k := tableKey{c.Schema, c.Table}
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], c)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].schema != keys[j].schema {
			return keys[i].schema < keys[j].schema
		}
		return keys[i].table < keys[j].table
	})

	var b strings.Builder
	for _, k := range keys {
		fullName := k.table
		if k.schema != "" && k.schema != "public" && k.schema != s.Database {
			fullName = k.schema + "." + k.table
		}
		fmt.Fprintf(&b, "TABLE %s (\n", fullName)
		for i, col := range groups[k] {
			fmt.Fprintf(&b, "  %s %s", col.Name, col.Type)
			if i < len(groups[k])-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(")\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
