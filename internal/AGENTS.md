# internal/

The core: one package per concern, no tenant, project or service names (root rule 1). Read a package's own `AGENTS.md` first.

## Where things are

| Package | Owns |
|---|---|
| `book` | schema, sqlc queries, goose migrations (expand-only) |
| `config` | instance and tenant YAML, file secrets |
| `flow` | state machine, CAS transitions, task budgets |
| `dispatch` | River jobs, scheduler, provider router |
| `jobsapi` | jobs API for the worker and `ns` |
| `runner` | worktree, role run, checks, patch |
| `harness` | `Harness` interface and its adapters |
| `check` | `nightshift check`, doc-lint |
| `policy` | blast radius, review policy, constitution |
| `train` | merge train, release, rollback |
| `gitops` | repo mirrors, push, PRs |
| `integration` | interfaces to external systems |
| `adapters` | tracker, docs, chat, github, checks; twins in `*/fake` |
| `testutil` | golden files, throwaway Postgres |

## Go rules

- Follow `standards/industry/go/style.md`; `.golangci.yml` enforces it.
- No abstraction, option or package before a second real caller needs it.
- Wrap errors with context, `context.Context` first, no mutable global state; log with `log/slog`, trace with OpenTelemetry.
- Comments say why; exported names get docs.

## Tests

- No models, network or tokens in tests: table-driven, `go test -fuzz` for parsers, rapid for state machines.
- Postgres only through `testutil/pgtest`; integration tests use real Postgres and run in CI.
- Scenarios replay `testdata/scenarios/*.yaml` on the `fake` harness and compare book events with golden files (`-update`, then review the diff).
- Levels: logic T1–T2; flow change a T4 scenario; deploy, runner or egress change T6 escape tests.
- No real model APIs; harness contract tests (T8) are opt-in.
