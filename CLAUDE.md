# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

`rdq` ("RDS Data Query") is a Go CLI built on the Kong framework that is
intended to wrap the AWS RDS Data API for Aurora. The entry point is
`cmd/rdq/main.go`, which dispatches to one of four subcommands under
`command/`: `exec`, `ask`, `gui`, `tui` (with `tui` marked
`default:"1"`, so bare `rdq` lands there).

The GUI mode launches an embedded HTTP server that serves a React + Vite
SPA out of the Go binary. That is the only subcommand with real logic
today — see "Implementation status" below before treating the README as
a source of truth.

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

Go toolchain is `go 1.25.6` (per `go.mod`). No tests exist yet; when
added, the standard `go test ./...` (or `go test ./command/...` for a
single package) applies. There is no custom linter beyond `go vet`.

## Architecture

Four layers, kept deliberately small:

1. **CLI layer** — `cmd/rdq/main.go` declares the Kong struct with
   global flags (`--profile`, `--debug`) and the four subcommands,
   constructs a shared `command.Globals`, and calls `ctx.Run(globals)`.
2. **Command layer** — `command/` has one file per subcommand, each
   exposing a `*Cmd` struct with a `Run(globals *Globals) error`
   method. `globals.go` defines the shared `Globals` struct passed to
   every command.
3. **Embedded server** — `internal/server/` is the backend for GUI
   mode:
   - `embed.go` uses `//go:embed all:dist` to embed a `dist` directory
     that lives **inside `internal/server/`**, not `frontend/dist`.
     The Makefile copies the Vite output into this location before the
     Go build runs — this coupling is the single most important build
     detail in the repo.
   - A single tracked `internal/server/dist/.gitkeep` exists so that
     `//go:embed all:dist` has at least one matching file when the
     module is fetched via `go install` (where `frontend/dist` is
     never built). `.gitignore` excludes everything else under that
     directory with an `internal/server/dist/*` + `!.../.gitkeep`
     pair, and `make frontend-build` uses
     `find ... ! -name '.gitkeep' -delete` so the placeholder is
     preserved across rebuilds. Do not remove `.gitkeep` or the
     `go install` path breaks.
   - `server.go` registers `/api/health` and serves the embedded SPA
     at `/` via `http.FileServer(http.FS(fsys))`.
4. **Frontend** — `frontend/` is a standard Vite + React SPA. Its build
   output is the payload that gets embedded into the Go binary.

The only command that currently wires these layers together end-to-end
is `command/gui.go`: it optionally opens a browser in a goroutine and
then blocks on `server.Run(port)`.

## Implementation status (important)

The README advertises features that are **not implemented yet**. When
asked to modify or extend any of the following, expect to be writing
the feature from scratch rather than editing existing logic:

- `command/exec.go`, `command/ask.go`, `command/tui.go` are stubs that
  only print via `fmt.Println`. No AWS SDK calls, no Bedrock
  integration, no TUI runtime.
- `go.mod` only depends on `github.com/alecthomas/kong`. The AWS SDK
  for Go v2, Bubble Tea, Bubbles, go-fuzzyfinder, and Bedrock
  integration mentioned in the README are not present.
- GUI mode is the only real code path today: Go HTTP server, embedded
  React SPA, single `/api/health` endpoint.

Other gotchas:

- Do not hand-edit `internal/server/dist/` — it is build output and
  will be overwritten by `make frontend-build`.
- The `rdq` binary at the repo root (~11 MB) is a build artifact.
  Rebuild via `make build` rather than relying on it.
