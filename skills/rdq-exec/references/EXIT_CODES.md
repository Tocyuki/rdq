# `rdq exec` exit codes — full reference

Source of truth: [`command/exec.go`](https://github.com/Tocyuki/rdq/blob/main/command/exec.go) constants `exitSuccess` … `exitTimeout`.

| Code | Constant | Trigger | Stdout | Stderr | Recovery |
| ---: | --- | --- | --- | --- | --- |
| 0 | `exitSuccess` | Statement ran and result rendered | Result in chosen format | `(N rows affected)` for writes | Use the result |
| 1 | `exitError` | AWS error, SQL error, render failure, or any non-categorized error | Empty | `rdq exec: <error>` | Read the error; do not retry without diagnosing |
| 2 | `exitUsage` | No SQL provided, both positional + `--file`, empty SQL after read, unsupported `--output` | Empty | `rdq exec: <usage hint>` | Fix the invocation |
| 3 | `exitReadOnly` | Profile is read-only and SQL is a write | Empty | `rdq exec: writes are blocked in read-only mode` + hint | **User must toggle read-only off** via `rdq tui` (`F8`) or GUI Settings; do not auto-flip |
| 4 | `exitNotConfirmed` | Destructive SQL (DELETE/UPDATE without WHERE, TRUNCATE) without `--yes` in non-TTY, or user answered "n" in TTY | Empty | `rdq exec: destructive statement requires --yes...` or `aborted by user` | Show user the SQL, get consent, re-run with `-y` |
| 5 | `exitTimeout` | AWS request exceeded `runner.ExecuteTimeout` | Empty | `rdq exec: context deadline exceeded` | Narrow the query; the Data API has its own timeout under heavy contention |

## Mapping AWS error → exit 1

`rdq exec` does **not** translate Aurora SQLSTATE codes into distinct exit
codes today. Common Aurora errors that surface as exit 1:

- `BadRequestException: SqlState ...` — bad SQL or missing privilege
- `BadRequestException: Database "X" not found` — wrong `--database`
- `BadRequestException: HttpEndPoint is not enabled` — Data API not enabled
  on the cluster (user fix in RDS console)
- `BadRequestException: Aurora resource ARN ... not found` — wrong cluster ARN
- `ResourceNotFoundException` — secret ARN typo or deleted secret
- Any `*credentials*` or `*signing*` error — AWS auth chain issue, fix
  with `AWS_PROFILE=` or `rdq -p`

## Decision tree for handling failures

```
            exec finished
                │
        ┌───────┴───────┐
   exit 0 / 1         exit 3 / 4 / 5
        │                  │
   stdout +/-          do NOT retry
   stderr msg          without user
        │              guidance
   render to user
```

- **Exits 3 and 4 are policy gates, not errors.** They mean "the user explicitly
  configured this guardrail; respect it."
- **Exit 5** indicates the Data API itself was slow. Suggest the user either
  narrow the query or run it from the TUI where they can watch progress.
- **Exit 1 with no obvious cause** → ask the user to retry with `--debug`;
  the AWS identity log helps spot wrong-account / wrong-region situations.
