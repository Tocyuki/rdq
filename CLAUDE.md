# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

`rdq` ("RDS Data Query") is a Go CLI built on the Kong framework that
wraps the AWS RDS Data API for Aurora. The entry point is
`cmd/rdq/main.go`, which dispatches to one of four subcommands under
`command/`: `exec`, `ask`, `gui`, `tui` (with `tui` marked
`default:"1"`, so bare `rdq` lands there).

`tui` is the primary mode and carries the bulk of the implementation:
a Bubble Tea app under `internal/tui/` that integrates AWS Data API,
Secrets Manager, RDS cluster discovery, SQL history, schema caching,
and Amazon Bedrock (Converse API) for AI-assisted query generation /
review / analysis.

`gui` launches a secondary mode: an embedded HTTP server that serves
a React + Vite SPA out of the Go binary. It is intentionally much
smaller than the TUI today.

`exec` and `ask` are placeholder stubs (`fmt.Printf` only) and will
be wired to the same engines as the TUI in a future release. Treat
them as write-from-scratch when asked to extend them.

`README.md` is currently an accurate source of truth for user-facing
features and keybindings — prefer it for behavior questions.

## Build & run commands

Top-level build is driven by the `Makefile`:

- `make build` — full build: frontend build, copy of `frontend/dist` to
  `internal/server/dist`, then `go build -o rdq ./cmd/rdq/`.
- `make frontend-build` — `cd frontend && npm run build`, then copies
  the output into `internal/server/dist` so Go's `//go:embed` picks it
  up. Run this any time `frontend/` changes.
- `make go-build` — Go-only build. Will embed **stale or missing**
  assets if `internal/server/dist` is not current; prefer `make build`
  when in doubt.
- `make dev` — `cd frontend && npm run dev` (Vite dev server; the Go
  binary is not involved).
- `make clean` — removes the `rdq` binary and both `dist` directories.

Inside `frontend/` (React 19 + TypeScript + Vite 8 + ESLint 9):

- `npm run dev` — Vite dev server.
- `npm run build` — `tsc -b && vite build`.
- `npm run lint` — `eslint .`.
- `npm run preview` — Vite preview of the built SPA.

Go toolchain is `go 1.25.6` (per `go.mod`). Tests live alongside the
packages they cover — run `go test ./...` for the whole module, or
narrow to a single package e.g. `go test ./internal/tui/...`. Most
coverage today is in `internal/tui/`, `internal/state/`,
`internal/history/`, `internal/schema/`, `internal/bedrock/`, and
`internal/awsauth/`. There is no custom linter beyond `go vet`.

## Architecture

Three layers, with most of the code living in the third:

1. **CLI layer** — `cmd/rdq/main.go` declares the Kong struct with
   global flags (`--profile`/`-p`, `--cluster`, `--secret`,
   `--database`, `--bedrock-model`, `--bedrock-language`, `--debug`),
   handles "pass-a-flag-without-a-value → interactive picker"
   semantics via `preScanBareFlags`, resolves the active profile and
   pre-resolves `cluster`/`secret`/`database` into a
   `command.Globals` before handing off to the subcommand. Also owns
   the friendly credentials-error message path.

2. **Command layer** — `command/` has one file per subcommand
   (`exec.go`, `ask.go`, `gui.go`, `tui.go`), each exposing a `*Cmd`
   struct with a `Run(globals *Globals) error` method.
   `globals.go` defines the shared `Globals` struct. The real command
   bodies are tiny — they call into `internal/` packages.

3. **Internal packages** (`internal/`) — where the real logic lives:

   | Package | Role |
   | --- | --- |
   | `awsauth/` | SDK config loading, profile enumeration + pre-TUI picker (`go-fuzzyfinder`), STS identity verification |
   | `bedrock/` | Amazon Bedrock Converse client + prompt templates (`prompt.go`) |
   | `connection/` | Aurora cluster / Secrets Manager / database discovery + selection (`cluster.go`, `secret.go`, `database.go`) |
   | `history/` | Per-profile SQL history persisted as JSONL with favourites |
   | `schema/` | `information_schema` fetch + on-disk cache keyed by (cluster, database) |
   | `server/` | GUI mode backend: `embed.go` (`//go:embed all:dist`) + `server.go` (`/api/health` + `http.FileServer`) |
   | `state/` | Per-profile state cache (`state.json`): cluster / secret / database / bedrock model + language / cluster→secret map / database history |
   | `tui/` | Bubble Tea app. `tui.go` entry, `model.go` the main state machine, `keys.go` all keybindings, `ai.go` Bedrock integration, `result.go` result viewer + row inspector, `export.go` CSV export, `highlight.go` SQL/JSON syntax highlighting via chroma. Tests: `model_test.go`, `result_test.go`, `export_test.go`, `highlight_test.go` |

   `frontend/` is a standalone Vite + React SPA. Its `npm run build`
   output is copied into `internal/server/dist/` by the Makefile and
   becomes the payload for `//go:embed all:dist` in the GUI mode only.

### Server dist embedding — the one build trap to remember

`internal/server/embed.go` uses `//go:embed all:dist` on a `dist`
directory that lives **inside `internal/server/`**, not
`frontend/dist`. The Makefile copies the Vite output into this
location before the Go build runs — this coupling is the single most
important build detail in the repo.

A single tracked `internal/server/dist/.gitkeep` exists so that
`//go:embed all:dist` has at least one matching file when the module
is fetched via `go install` (where `frontend/dist` is never built).
`.gitignore` excludes everything else under that directory with an
`internal/server/dist/*` + `!.../.gitkeep` pair, and `make
frontend-build` uses `find ... ! -name '.gitkeep' -delete` so the
placeholder is preserved across rebuilds. **Do not remove `.gitkeep`
or the `go install` path breaks.**

## Implementation status (important)

| Subcommand / feature | Status |
| --- | --- |
| `rdq` (bare, defaults to TUI) | ✅ Implemented |
| `rdq tui` | ✅ Implemented (Bubble Tea app in `internal/tui/`) |
| `rdq gui` | ✅ Implemented (React SPA served from embedded HTTP server) |
| `rdq exec <sql>` (one-shot CLI) | 🚧 Stub — `command/exec.go` just `fmt.Printf`s |
| `rdq ask <prompt>` (one-shot CLI) | 🚧 Stub — `command/ask.go` just `fmt.Printf`s |
| Vim mode editor in TUI | 🚧 Planned |

When asked to extend `rdq exec` or `rdq ask`, expect to write the
feature from scratch. The TUI already has all the engines (Data API
execution, result rendering, Bedrock integration) — you can reuse
them rather than reimplement them; look at `internal/tui/runner.go`,
`internal/tui/result.go`, `internal/tui/ai.go`, and
`internal/bedrock/` for the pieces.

Other gotchas:

- Do not hand-edit `internal/server/dist/` — it is build output and
  will be overwritten by `make frontend-build`. Leave `.gitkeep` in
  place (see "Server dist embedding" in Architecture).
- The `rdq` binary at the repo root is a build artifact. Rebuild via
  `make build` (full) or `make go-build` (Go-only, keeps the existing
  `internal/server/dist/` contents) rather than relying on what is
  committed.
- State / history / schema files live under `~/.rdq/` by default.
  Tests that touch these paths should use the env overrides
  (`RDQ_STATE_FILE`, `RDQ_HISTORY_FILE`, `RDQ_SCHEMA_DIR`) to avoid
  polluting the developer's real state. See
  `internal/state/state.go`, `internal/history/history.go`,
  `internal/schema/schema.go` for where each env var is read.
- The in-TUI picker filter in `internal/tui/model.go` (`containsFilter`)
  is a deliberate custom substring matcher, not fuzzy — `sahilm/fuzzy`
  is listed as indirect in `go.mod` only because `bubbles/list` pulls
  it in. Do not "restore" fuzzy matching without reading the comment
  explaining why it was replaced.
