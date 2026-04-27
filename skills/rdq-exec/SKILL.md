---
name: rdq-exec
description: Execute SQL one-shot against AWS Aurora via the rdq CLI (RDS Data API). Use when the user asks to run a SELECT/INSERT/UPDATE/DELETE/DDL statement against a configured Aurora cluster, fetch query results as table/JSON/CSV, or pipe SQL output into another tool. Honors rdq's per-profile read-only guard and destructive-statement confirmation, so the agent cannot accidentally write to production. Skill is for non-interactive use; for interactive exploration the user should run `rdq tui` themselves.
license: MIT
metadata:
  product: rdq
  upstream: https://github.com/Tocyuki/rdq
allowed-tools: Bash(rdq:*)
---

# rdq exec — one-shot SQL against Aurora over the RDS Data API

`rdq exec` runs a single SQL statement against an Aurora cluster via AWS RDS
Data API and prints the result. It is the **non-interactive surface** of `rdq`,
designed for scripts, CI pipelines, and AI agents.

## When to use this skill

Trigger on requests like:

- "Run this query against Aurora and show me the result"
- "How many rows does the `orders` table have right now?"
- "Export the result of `SELECT ...` as CSV"
- "Update one row in the dev database" (writes — see safety section)

**Do not** use this skill for interactive exploration, schema browsing across
many statements, or anything requiring multi-step UI. Direct the user to
`rdq tui` (TUI) or `rdq gui` (browser-based client) instead.

## Prerequisites

The user must have already configured a connection. Verify with:

```bash
rdq exec --help
```

If the user has never run `rdq` before, ask them to launch `rdq tui` once to
pick AWS profile / cluster / secret / database — those choices persist to
`~/.rdq/state.json` and `rdq exec` reuses them automatically. Alternatively
they can pass `--profile`, `--cluster`, `--secret`, `--database` explicitly.

## Synopsis

```
rdq exec [global-flags] [-f FILE | -] [-o table|json|csv] [-y] [SQL]
```

`SQL` (positional) and `--file` are **mutually exclusive**. Exactly one of:
positional `SQL`, `--file <path>`, or `--file -` (stdin) must be provided.

## Arguments

| Argument | Default | Description |
| --- | --- | --- |
| `SQL` (positional) | — | The SQL statement. Mutually exclusive with `--file`. |
| `-f`, `--file <path>` | — | Read SQL from a file. Use `-` for stdin. |
| `-o`, `--output <fmt>` | `table` | One of `table`, `json`, `csv`. |
| `-y`, `--yes` | `false` | Skip the destructive-statement confirmation prompt. **Required in non-interactive shells (this skill) for DELETE/UPDATE without WHERE and TRUNCATE.** |

Inherited global flags (rarely needed in this skill — defaults from
`~/.rdq/state.json` are usually correct):

| Flag | Description |
| --- | --- |
| `-p`, `--profile <name>` | AWS profile to use. |
| `--cluster <arn>` | Aurora cluster ARN. |
| `--secret <arn>` | Secrets Manager secret ARN with the DB credentials. |
| `--database <name>` | Database name to query. |
| `-d`, `--debug` | Verbose AWS identity log on stderr. |

Bedrock-related global flags (`--bedrock-model`, `--bedrock-language`) do not
apply to `exec`; they are for the TUI's natural-language Ask feature.

## Output channels

- `--output table|json|csv` → **stdout**. Safe to pipe into `jq`, `column`,
  another command, or redirect to a file.
- `(N rows affected)` for write statements → **stderr**. This keeps stdout a
  clean data stream so `rdq exec '...' -o json | jq ...` always works.
- All errors → **stderr**, prefixed `rdq exec: `.

### JSON shape

```json
{
  "columns": ["id", "email", "created_at"],
  "rows": [
    [1, "alice@example.com", "2025-01-02T03:04:05Z"],
    [2, "bob@example.com",   null]
  ],
  "updated": -1
}
```

`updated >= 0` means the statement was a write (INSERT / UPDATE / DELETE);
the value is the affected row count. `updated == -1` means it was a read.
NULL DB values become JSON `null`. Binary blobs become `"<blob N bytes>"`
strings (the Data API does not return raw bytes for ad-hoc queries).

### CSV shape

RFC 4180 compliant. NULL → empty cell; blobs → `<blob N bytes>`; numeric
columns are not quoted. No trailing newline after the last row.

## Exit codes

| Code | Meaning | Agent action |
| --- | --- | --- |
| `0` | Success | Read stdout |
| `1` | General error (SQL syntax, AWS API error, render error) | Read stderr; do not retry blindly |
| `2` | Usage error (no SQL given, both arg and `--file`, etc.) | Fix invocation and retry |
| `3` | Read-only mode blocked the write | **Stop. See "Read-only mode" below.** |
| `4` | User declined / `--yes` missing for destructive SQL | See "Destructive-statement confirmation" |
| `5` | AWS request timed out | Consider narrowing the query; do not retry without telling the user |

See [`references/EXIT_CODES.md`](references/EXIT_CODES.md) for full details
and recovery patterns.

## Safety: read-only mode

Each AWS profile has a tri-state read-only flag in `~/.rdq/state.json`:

- **Unset (fresh profile)** → defaults to **read-only ON**.
- **Explicit ON** → only `SELECT`, `WITH`, `SHOW`, `EXPLAIN`, `DESCRIBE`,
  `DESC`, `TABLE`, `VALUES` may execute.
- **Explicit OFF** → all statements allowed.

When read-only is ON and the SQL is a write, `rdq exec` exits **3** without
contacting AWS. The agent **must not** disable this on the user's behalf —
only the user can flip it via `rdq tui` (`F8`) or the GUI Settings panel.

If you hit exit 3, surface this to the user with a message like:

> rdq is in read-only mode for this profile. To allow writes, run `rdq tui`
> and toggle with `F8`, or open the GUI Settings, then re-run.

## Safety: destructive-statement confirmation

These statements always require confirmation regardless of the read-only
flag:

- `DELETE` without a `WHERE` clause
- `UPDATE` without a `WHERE` clause
- `TRUNCATE`
- `WITH ... DELETE/UPDATE` without a `WHERE` (conservative default)

The classifier strips comments and string literals first, so
`UPDATE t SET note = '-- not a comment'` is correctly classified as
"missing WHERE".

In an interactive TTY the user gets a `[y/N]` prompt. **In a non-interactive
shell (this skill's environment) you must pass `--yes`** or `rdq exec` will
exit **4** without doing anything. Always show the user the exact statement
and the consequence before adding `--yes`.

## Examples

### Simple read

```bash
rdq exec "SELECT id, email FROM users WHERE created_at > NOW() - INTERVAL '7 days' LIMIT 10"
```

### JSON pipeline

```bash
rdq exec "SELECT count(*) AS n FROM orders" -o json | jq '.rows[0][0]'
```

### Heredoc / multi-statement file

```bash
rdq exec -f migrations/2026-04-add-index.sql
```

The Data API accepts a single statement per call. If the file contains
multiple `;`-separated statements, run them one at a time (the user should
split the file or invoke `rdq exec` in a loop).

### Pipe SQL via stdin

```bash
echo "SELECT current_database()" | rdq exec -f -
```

### Targeted write (with explicit safety)

```bash
# DELETE WITH a WHERE clause — does NOT trigger the destructive prompt.
rdq exec "DELETE FROM sessions WHERE expires_at < NOW() - INTERVAL '30 days'"
```

```bash
# Wide TRUNCATE — requires --yes in non-interactive mode. Always confirm
# with the user first; never add --yes silently.
rdq exec --yes "TRUNCATE staging_imports"
```

### Cross-profile one-off

```bash
rdq exec -p staging "SELECT version()"
```

This uses the `staging` AWS profile; cluster/secret/database resolve from
`~/.rdq/state.json`'s cached selection for that profile.

## Common errors and what they mean

| Error message starts with | Cause | Fix |
| --- | --- | --- |
| `rdq exec: provide SQL as a positional argument...` | Neither arg nor `--file` given | Pass SQL one way |
| `rdq exec: positional SQL and --file are mutually exclusive` | Both given | Pick one |
| `rdq exec: empty SQL` | File or stdin had only whitespace | Provide a real statement |
| `rdq exec: writes are blocked in read-only mode` | exit 3 | See "Read-only mode" |
| `rdq exec: destructive statement requires --yes...` | exit 4 in non-TTY | Confirm with user, then add `-y` |
| `rdq: no AWS credentials available.` | SDK chain returned nothing | User must configure `AWS_PROFILE`, env vars, or run `rdq -p` |
| `BadRequestException: ... database "..." does not exist` | `--database` mismatch | Check `~/.rdq/state.json` or pass `--database` explicitly |

## Decision rules for the agent

1. **Default to `-o json`** when the result will be processed further — it is
   the only stable machine-readable format.
2. **Use `-o table`** only when the user explicitly asked to "see" or
   "show" the result.
3. **Never silently add `-y`.** Show the SQL, explain what it will do, get
   user consent, then re-run with `-y`.
4. **Never disable read-only mode for the user.** It is a deliberate guardrail.
5. **Treat exit 3 and 4 as user-action-required, not retryable errors.**
6. **Quote SQL with single quotes in shell** to keep `$`, backticks, and `!`
   inert: `rdq exec 'SELECT $1 FROM t'`.

## See also

- [`references/EXIT_CODES.md`](references/EXIT_CODES.md) — every exit code,
  cause, and recovery pattern in one place.
- `rdq --help` — global flags and other subcommands (`tui`, `gui`, `ask`).
