# internal/

The core: one package per concern, no tenant, project or service names (root rule 1). Read a package's own `AGENTS.md` first.

## Where things are

| Package | Owns |
|---|---|
| `flow` | state machine, CAS transitions, task budgets |
| `dispatch` | River jobs, scheduler, provider router |
| `assurance/*` | checks, policy, standards, rubrics |
| `data/book` | schema, sqlc queries, goose migrations (expand-only) |
| `customization/*` | instance and tenant YAML, stack packs, onboarding |
| `train` | merge train, release, rollback |
| `execution/*` | `Harness` and its adapters, worker runner, jobs API, checkouts and diffs, toolchain, skills, deployer |
| `trustedgit` | pushes, PRs and merges with the factory's token |
| `integration` | interfaces to external systems |
| `adapters` | tracker, docs, chat, github, checks; twins in `*/fake` |
| `testutil` | golden files, throwaway Postgres |

Packages move into plane folders `internal/<plane>/<package>` (ADR 0026); each folder's `AGENTS.md` lists its packages. Any other package says what it owns in its package comment.

## Go rules

- Follow `standards/industry/go/style.md`; `.golangci.yml` enforces it.
- No abstraction, option or package before a second real caller needs it.
- Wrap errors with context, `context.Context` first, no mutable global state; log with `log/slog`, trace with OpenTelemetry.
- Comments say why; exported names get docs.

## Tests

- No models, network or tokens in tests: table-driven, `go test -fuzz` for parsers, rapid for state machines.
- Postgres only through `testutil/pgtest`; integration tests use real Postgres and run in CI.
- Scenarios replay `testdata/scenarios/*.yaml` on the `fake` harness and compare book events with golden files (`-update`, then review the diff).
- Harness contract tests against a real CLI (T8) are opt-in.
