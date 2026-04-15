# rdq

[![CI](https://github.com/Tocyuki/rdq/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Tocyuki/rdq/actions/workflows/ci.yml)

**RDS Data Query** — A terminal UI for querying Aurora over the AWS RDS Data API, with built-in Amazon Bedrock assistance.

> [!WARNING]
> This project is under active development. Features and keybindings may change without notice.

## Overview

`rdq` is a Go-based TUI that wraps the AWS RDS Data API (`ExecuteStatement`) so you can:

- Edit SQL in a multi-line editor and run it against Aurora over HTTPS (no VPC / bastion required)
- See results as a table or JSON, drill into a single row, export to CSV, or yank to the clipboard
- Generate SQL from natural language with Amazon Bedrock, with multi-turn chat history within a session
- Get automatic, in-context error explanations whenever a query fails
- Switch between AWS profiles, clusters, secrets, Bedrock models, and response languages without leaving the TUI

The `gui` subcommand exposes a separate browser-based SQL client (React + Vite SPA embedded in the binary). `exec` and `ask` are placeholder stubs and will be wired to the same engines as the TUI in a future release.

## Features

### TUI mode (default)

- **Multi-line SQL editor** with execute via `F5` or `^R`
- **Result viewer** with three sub-modes:
  - **Table view** — vim-style cursor: `j` / `k` for rows, `h` / `l` / `0` / `$` for columns, with horizontal scroll for wide tables and a `▸ ` marker on the active column
  - **JSON view** (`^J` toggle) — viewport with `j` / `k` / `gg` / `G` and `h` / `l` / `0` / `$` for horizontal scroll
  - **Row inspector** (`Enter`) — preserves long values on a single line; scroll vertically with `j` / `k` / `gg` / `G` and horizontally with `h` / `l` / `0` / `$`; footer shows `line N/M`
- **Cursor position indicator** — table footer always shows `row N/M · col K/L <name>` so the user can tell where they are
- **vim-style yy yank** copies the current view (table CSV / result JSON / row JSON / explanation) to the system clipboard with an auto-clearing flash confirmation (~2.5 s)
- **CSV export** (`^E`) writes the current result to a timestamped file in cwd
- **SQL history** stored per profile, recallable via the history picker (`^H`) with incremental substring filter. **Favourites**: press `^F` inside the picker to mark / unmark an entry; favourites float to the top of the list and survive across runs

### AI integration (Amazon Bedrock)

- **Ask AI** (`^G`) — natural-language prompt → SQL **replaces** the editor contents. Multi-turn chat history is preserved within a session, so follow-ups like "now sort by created_at desc" inherit context. The chat resets when you switch cluster or profile (the previous schema no longer applies). Failed prompts are automatically removed from the history so retries do not duplicate turns
- **F6 = unified review / analyze / explain** — picks the right action based on focus + screen state:
  - Editor focus + non-empty SQL → **review the SQL** (correctness / performance / safety / style)
  - Results focus + result rows → **analyze the result** (counts, distributions, outliers)
  - Results focus + error → **explain the error** (root cause + fix, with the verbatim DB error always shown above the analysis)
- **Background schema fetch** — `information_schema` is fetched and cached so AI prompts can resolve real table / column names without re-querying
- **Per-profile model + language** — choose a Bedrock inference profile / foundation model (`^O`) and a response language (`^L` — Japanese / English / Chinese / Korean / Spanish / French) once and they are remembered

### Connection management

- **AWS profile picker** (`^P`) — incremental substring search over `~/.aws/config` profiles, switch the active credentials without restarting; rebuilds the SDK clients and reloads the cached connection for the new profile
- **Cluster picker** (`^T`) — list Aurora clusters with the Data API enabled in the active region
- **Secret picker** (`^\`) — switch the secret used for the current cluster (read-only ↔ admin etc.) without going through the cluster picker first
- **Automatic cluster→secret resolution** — when picking a cluster the matching secret is found via, in order:
  1. Per-profile cluster→secret cache in `~/.rdq/state.json`
  2. The cluster's `MasterUserSecret` (RDS-managed password)
  3. Secrets tagged `aws:rds:primaryDBClusterArn = <cluster ARN>`
  4. Falling back to a region-wide secret picker
- **Persistent state** — profile / cluster / secret / database / Bedrock model / language are cached so subsequent launches skip the prompts
- **Ephemeral mode for direct credentials** — when no profile name is in play (`AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY` only), `rdq` walks through the cluster / secret / database pickers from scratch every launch and writes nothing to `~/.rdq/state.json` or the history log. The status bar shows `(direct credentials · ephemeral)` so the mode is visible
- **Friendly credentials error** — if no provider in the SDK chain can produce credentials, `rdq` exits with an actionable message instead of the raw SDK error

## Installation

```bash
go install github.com/Tocyuki/rdq/cmd/rdq@latest
```

Or build from source:

```bash
git clone https://github.com/Tocyuki/rdq.git
cd rdq
go build -o rdq ./cmd/rdq/
```

## Usage

### Quick start

```bash
rdq
```

`rdq` with no arguments launches the TUI. The first run walks you through profile / cluster / secret / database selection. Subsequent runs jump straight in using the cached connection.

### Global flags

| Flag | Short | Description |
| --- | --- | --- |
| `--profile` | `-p` | AWS profile (falls back to `AWS_PROFILE`). Pass without a value for an interactive picker. |
| `--cluster` | | Aurora cluster ARN. Pass without a value for an interactive picker. |
| `--secret` | | Secrets Manager secret ARN. Pass without a value for an interactive picker. |
| `--database` | | Database name. Pass without a value to pick from history or enter manually. |
| `--bedrock-model` | | Override the cached Bedrock model ID (env: `RDQ_BEDROCK_MODEL`). |
| `--bedrock-language` | | Override the cached response language (env: `RDQ_BEDROCK_LANGUAGE`). |
| `--debug` | `-d` | Verbose logging. |

### GUI subcommand flags

Only applies to `rdq gui`. The TUI uses neither flag.

| Flag | Short | Default | Description |
| --- | --- | --- | --- |
| `--port` | `-P` | `8080` | Port the embedded HTTP server listens on. |
| `--no-open` | | `false` | Skip opening the browser automatically on launch. |

### TUI keybindings

#### Global

| Key | Action |
| --- | --- |
| `F5` / `^R` | Execute SQL |
| `Tab` | Move focus between editor and results pane |
| `^J` | Toggle table / JSON view |
| `Enter` | Open row inspector (in table view) / close inspector |
| `^G` | Ask AI — open natural-language prompt input (always SQL generation) |
| `F6` | Review / analyze / explain — picks the right one from current focus + state |
| `^P` | Switch AWS profile |
| `^T` | Switch cluster |
| `^\` | Switch secret for the current cluster |
| `^O` | Switch Bedrock model |
| `^L` | Switch Bedrock response language |
| `^H` | SQL history picker (substring filter; `^F` toggles favourite on selected entry) |
| `^E` | Export the current result to CSV in the working directory |
| `Esc` | Clear error / close current overlay |
| `?` | Toggle full help |
| `^C` | Quit |

#### Table view (results focus)

| Key | Action |
| --- | --- |
| `j` / `↓` | Next row |
| `k` / `↑` | Previous row |
| `l` / `→` | Next column (with horizontal scroll for wide tables) |
| `h` / `←` | Previous column |
| `0` / `Home` | Jump to first column |
| `$` / `End` | Jump to last column |
| `Enter` | Open row inspector |
| `yy` | Yank entire result as CSV |

#### Row inspector (after `Enter`)

| Key | Action |
| --- | --- |
| `j` / `k` / `↓` / `↑` | Scroll one line |
| `gg` / `G` | Jump to top / bottom |
| `h` / `l` / `←` / `→` | Horizontal scroll (4 cells) |
| `0` / `$` / `Home` / `End` | Jump to left / right edge |
| `yy` | Yank current row JSON |
| `Enter` / `Esc` | Close inspector |

Long values stay on a single line, so use `h` / `l` / `0` / `$` to scroll across wide JSON values — the same navigation as the JSON view.

#### JSON view (`^J`) and explanation overlay (after `F6` on an error)

| Key | Action |
| --- | --- |
| `j` / `k` | Scroll one line |
| `gg` / `G` | Jump to top / bottom |
| `h` / `l` | Horizontal scroll (4 cells) |
| `0` / `$` | Left edge / right edge |
| `yy` | Yank current view |
| `Esc` | Close overlay |

All pickers (profile / cluster / secret / model / language / history) support **type-to-filter incremental search** with substring matching. Selection happens with `Enter`; `Esc` cancels. Inside the history picker, `^F` marks/unmarks an entry as a favourite (★).

### AI workflow examples

**Generate SQL from natural language**:

1. Press `^G` from the editor → enter a natural-language prompt → `Enter`
2. The generated SQL **replaces** the editor contents (the previous draft is preserved in `^H` history if you ran it)
3. Review and press `F5` to execute
4. Press `^G` again to refine: "now group by month and sort desc" — the model sees the previous turn

**Review the SQL you just wrote**:

1. With the editor focused and a SQL statement on screen, press `F6`
2. An optional **focus area** prompt appears — type something like `performance` or `index usage` to narrow the review, or press `Enter` with an empty prompt for a general review
3. The model returns a markdown review (correctness / performance / safety / style) inside the result pane

**Analyze a query result**:

1. Run a query that returns rows
2. Press `Tab` to focus the results pane
3. Press `F6` → an optional focus area prompt appears (same as review); press `Enter` with an empty prompt for a general summary, or type something like `outliers` to narrow the analysis
4. The model summarises counts, distributions, outliers and surfaces notable patterns

**Explain an error**:

1. Run a SQL statement that fails — the error is shown in the results pane
2. Press `Tab` to focus results, then `F6`
3. The model returns `## Database error` (verbatim DB error) + `## Analysis` (root cause + suggested fix)

### Cluster ↔ Secret matching

If your cluster uses RDS-managed master passwords (`MasterUserSecret`), the matching secret is selected automatically. If your secrets are managed by Terraform / IaC and the picker still asks every time, simply select the right secret once and `rdq` will remember the pairing in `~/.rdq/state.json` for next time.

### Direct credentials (ephemeral mode)

When you start `rdq` without a profile name in play (no `--profile`, no `AWS_PROFILE`, but `AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY` are exported), the TUI runs in **ephemeral mode**:

- Status bar shows `(direct credentials · ephemeral)`
- Cluster / secret / database pickers run from scratch every launch
- Nothing is written to `~/.rdq/state.json` or the SQL history file
- Switching to a named profile from inside the TUI (`^P`) re-enables persistence

## State files

| Path | Purpose |
| --- | --- |
| `~/.rdq/state.json` | Per-profile cache: cluster ARN, secret ARN, database, Bedrock model, response language, cluster→secret map, database history |
| `~/.rdq/history.jsonl` | SQL execution history (append-only JSONL); each entry stores its profile + database so the picker can filter |
| `~/.rdq/schema/<hash>.json` | Cached `information_schema` snapshot per (cluster, database) pair |

Override locations with `RDQ_STATE_FILE`, `RDQ_HISTORY_FILE`, `RDQ_SCHEMA_DIR` if needed.

In **ephemeral mode** (direct credentials, no profile name) none of these files are touched.

## AWS Profile resolution

`rdq` picks the active profile through the following branches:

| Invocation | Result |
| --- | --- |
| `rdq -p value` | Use `value` |
| `rdq -p` (no value) + `AWS_PROFILE=foo` | Use `foo` (skip the picker — env wins) |
| `rdq -p` (no value), env unset | Open the interactive fuzzy picker |
| `rdq` (no flag) + `AWS_PROFILE=foo` | Use `foo` |
| `rdq` (no flag) + `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (no `AWS_PROFILE`) | Use the SDK default credentials chain → **ephemeral mode** |
| `rdq` (no flag), nothing configured | Friendly error message + exit 1 |

Inside the TUI, `^P` opens the profile picker again at any time. Switching to a named profile from ephemeral mode re-enables persistence and history.

## Prerequisites

- AWS credentials configured (`~/.aws/credentials`, SSO, env vars, ...)
- IAM permissions:
  - **RDS** — `rds:DescribeDBClusters`
  - **Secrets Manager** — `secretsmanager:GetSecretValue`, `secretsmanager:ListSecrets`, `secretsmanager:DescribeSecret`
  - **RDS Data API** — `rds-data:ExecuteStatement`
  - **Bedrock** (optional, only for AI features) — `bedrock:Converse`, `bedrock:ListInferenceProfiles`, `bedrock:ListFoundationModels`
- An Aurora cluster with the [Data API enabled](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/data-api.html)

## Tech stack

| Component | Library |
| --- | --- |
| Language | Go 1.25 |
| AWS SDK | [aws-sdk-go-v2](https://github.com/aws/aws-sdk-go-v2) (rds, rdsdata, secretsmanager, sts, bedrock, bedrockruntime) |
| CLI framework | [Kong](https://github.com/alecthomas/kong) |
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Bubbles](https://github.com/charmbracelet/bubbles) + [Lipgloss](https://github.com/charmbracelet/lipgloss) |
| Pre-TUI fuzzy pickers | [go-fuzzyfinder](https://github.com/ktr0731/go-fuzzyfinder) |
| In-TUI picker filtering | Custom literal substring matcher injected into [bubbles/list](https://github.com/charmbracelet/bubbles) (deliberately chosen over fuzzy matching for predictable command-palette-style search) |
| Syntax highlighting | [chroma](https://github.com/alecthomas/chroma) |
| Clipboard | [atotto/clipboard](https://github.com/atotto/clipboard) |
| LLM | [Amazon Bedrock Converse API](https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference.html) |

## Implementation status

| Feature | Status |
| --- | --- |
| `rdq` (TUI default) | ✅ Implemented |
| `rdq tui` | ✅ Implemented |
| `rdq gui` | ✅ Implemented (separate React SPA, browser-based) |
| `rdq exec <sql>` (one-shot CLI) | 🚧 Stub |
| `rdq ask <prompt>` (one-shot CLI) | 🚧 Stub |
| Vim mode editor | 🚧 Planned |
| Visual selection / `dd` / `p` | 🚧 Planned |

## Development

```bash
git clone https://github.com/Tocyuki/rdq.git
cd rdq
make hooks            # one-time: install the pre-commit gofmt guard
make go-build         # Go-only build
./rdq --help

make fmt              # gofmt -w .
make check            # fmt-check + vet + test -race (mirrors CI)
```

`make check` is the exact set of steps the CI "Go" job runs, so a
green `make check` locally means a green CI. The `make hooks` target
installs a tiny pre-commit hook (`.githooks/pre-commit`) that runs
`gofmt -l` on any staged Go files — it catches the most common "red
CI" mistake before the commit lands without slowing down
`git commit` with test runs.

For the GUI mode (browser SPA), `make build` runs the Vite frontend build and embeds it into the Go binary. See `CLAUDE.md` for the layered architecture overview.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
