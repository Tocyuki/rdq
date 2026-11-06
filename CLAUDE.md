# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

`rdq` ("RDS Data API Query") is a Go CLI built on the Kong framework that
wraps the AWS RDS Data API for Aurora. The entry point is
`cmd/rdq/main.go`, which dispatches to one of three subcommands under
`command/`: `exec`, `ask`, `tui` (with `tui` marked
`default:"1"`, so bare `rdq` lands there). rdq is a TUI-focused tool;
there is no browser / GUI mode.

`tui` is the primary mode and carries the bulk of the implementation:
a Bubble Tea app under `internal/tui/` that integrates AWS Data API,
Secrets Manager, RDS cluster discovery, SQL history, schema caching,
and Amazon Bedrock (Converse API) for AI-assisted query generation /
review / analysis.

`exec` and `ask` are one-shot CLI modes that share the same
read-only / destructive-confirmation gates as the TUI. Their
real logic lives in `command/exec.go` and `command/ask.go`, with
shared engines under `internal/runner/`. Both surfaces are fully
implemented and have unit tests in `command/exec_test.go` and
`command/ask_test.go`.

`skills` is a subcommand handled outside Kong (early branch in
`cmd/rdq/main.go`) that delegates to `github.com/Songmu/skillsmith`.
It ships Agent Skills (agentskills.io spec) embedded in the binary
and lets users `rdq skills install` them into `~/.agents/skills/`
for AI agents (Claude Code, Copilot, Codex) to consume.

`README.md` is currently an accurate source of truth for user-facing
features and keybindings — prefer it for behavior questions.

## Build & run commands

Top-level build is driven by the `Makefile`:

- `make build` — `go build -o rdq ./cmd/rdq/`.
- `make clean` — removes the `rdq` binary.
- `make check` — `fmt-check` + `vet` + `test -race` (mirrors the CI
  "Go" job).

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
   (`exec.go`, `ask.go`, `tui.go`), each exposing a `*Cmd`
   struct with a `Run(globals *Globals) error` method.
   `globals.go` defines the shared `Globals` struct. The real command
   bodies are tiny — they call into `internal/` packages.

3. **Internal packages** (`internal/`) — where the real logic lives:

   | Package | Role |
   | --- | --- |
   | `awsauth/` | SDK config loading, profile enumeration + pre-TUI picker (`go-fuzzyfinder`), STS identity verification |
   | `bedrock/` | Amazon Bedrock Converse client + prompt templates (`prompt.go`) |
   | `connection/` | Aurora cluster / Secrets Manager / database discovery + selection (`cluster.go`, `secret.go`, `database.go`) |
   | `fsutil/` | Atomic file write (`WriteFileAtomic`) + inter-process lock (`WithLock`, flock / LockFileEx) shared by `state/`, `history/`, `schema/` so concurrent rdq processes (TUI + CLI) never lose each other's writes |
   | `history/` | Per-profile SQL history persisted as JSONL with favourites |
   | `runner/` | RDS Data API execution (`runner.go`), result rendering (`result.go`, `render.go`), read-only classifier (`readonly.go`), destructive-statement gate (`confirm.go`). Shared by `command/exec.go`, `command/ask.go`, and `internal/tui/runner.go` so every surface applies identical policies. |
   | `schema/` | `information_schema` fetch + on-disk cache keyed by (cluster, database) |
   | `state/` | Per-profile state cache (`state.json`): cluster / secret / database / bedrock model + language / cluster→secret map / database history. Mutate via `state.UpdateProfile` (locked read-modify-write), not `Load`+`Save` |
   | `tui/` | Bubble Tea app. `tui.go` entry, `model.go` the main state machine, `keys.go` all keybindings, `ai.go` Bedrock integration, `result.go` result viewer + row inspector, `export.go` CSV export, `highlight.go` SQL/JSON syntax highlighting via chroma. Tests: `model_test.go`, `result_test.go`, `export_test.go`, `highlight_test.go` |

### Skill distribution — `skills/` and the early Kong bypass

Agent Skills (agentskills.io spec) are embedded via `skills/embed.go`
(`//go:embed all:rdq-exec`) and consumed by `cmd/rdq/skills.go`. Two
non-obvious details:

1. **`skills/` lives at the repo root**, not under `internal/`. Go's
   `//go:embed` cannot traverse `..`, so the embed directive must sit
   in a package that is a sibling of the skill directories. Adding
   a new skill = create `skills/<name>/SKILL.md` and append `<name>`
   to the `//go:embed` line in `skills/embed.go`.
2. **The `skills` subcommand is dispatched before Kong** in
   `cmd/rdq/main.go` (`isSkillsInvocation` / `dispatchSkills`).
   Reasons: `rdq skills` does not need AWS credentials (Kong's
   pre-dispatch credential check would otherwise reject it), and
   skillsmith parses its own argv via the stdlib `flag` package, so
   running it through Kong would just be double-parsing.

`SKILL.md` frontmatter `name` must equal the parent directory name
(agentskills.io rule). Body is documentation aimed at AI agents, not
humans — keep it under ~500 lines and split detail into
`references/` for progressive disclosure.

Version string passed to `skillsmith.New` comes from
`runtime/debug.ReadBuildInfo` (`Main.Version`); local builds get the
fallback `"0.0.0-dev"`. tagpr-released binaries automatically pick up
their semver tag, which is what skillsmith stores in
`~/.agents/skills/<name>/.skillsmith.json`.

## Implementation status (important)

| Subcommand / feature | Status |
| --- | --- |
| `rdq` (bare, defaults to TUI) | ✅ Implemented |
| `rdq tui` | ✅ Implemented (Bubble Tea app in `internal/tui/`) |
| `rdq exec <sql>` (one-shot CLI) | ✅ Implemented (`command/exec.go` + `internal/runner/`) |
| `rdq ask <prompt>` (one-shot CLI) | ✅ Implemented (`command/ask.go`, layers Bedrock on top of exec) |
| `rdq skills ...` (Agent Skill distribution) | ✅ Implemented (`cmd/rdq/skills.go` + `skills/`) |
| Vim mode editor in TUI | 🚧 Planned |

When asked to extend `rdq exec` or `rdq ask`, expect to write the
feature from scratch. The TUI already has all the engines (Data API
execution, result rendering, Bedrock integration) — you can reuse
them rather than reimplement them; look at `internal/tui/runner.go`,
`internal/tui/result.go`, `internal/tui/ai.go`, and
`internal/bedrock/` for the pieces.

Other gotchas:

- The `rdq` binary at the repo root is a build artifact. Rebuild via
  `make build` rather than relying on what is committed.
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
