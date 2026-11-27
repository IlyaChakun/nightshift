# internal/

The core: one package per concern, no tenant, project or service names (root rule 1). Read a package's own `AGENTS.md` first.

## Where things are

| Package | Owns |
|---|---|
| `experience/*` | doors: MCP tools for the inbox |
| `knowledge/*`, `evolution/*`, `observability/*` | search and memory, learning, summaries |
| `control/*` | chat, code host and tracker contracts, task flow, River jobs and outbox, merge train, deploy steps, switches, jobs server, trusted Git |
| `assurance/*` | checks, policy, standards, rubrics |
| `data/book` | schema, sqlc queries, goose migrations (expand-only) |
| `customization/*` | instance and tenant YAML, stack packs, onboarding |
| `execution/*` | `Harness` and its adapters, worker runner, jobs API, checkouts and diffs, toolchain, skills, deploy contract |
| `adapters` | tracker, docs, chat, github, checks; twins in `*/fake` |
| `testing/*` | eval selection and runs, golden files, throwaway Postgres, the architecture test |

Packages move into plane folders `internal/<plane>/<package>` (ADR 0026); each folder's `AGENTS.md` lists its packages. Any other package says what it owns in its package comment.

## Go rules

- Follow `standards/industry/go/style.md`; our `.golangci.yml` says how it differs from the tenant template.
- No abstraction before a second real caller, except contracts the design names for a later phase (chat, code host, tracker, harness, policy checks).
- Log with `log/slog`, trace with OpenTelemetry.

## Tests

- No models, network or tokens in tests: table-driven, `go test -fuzz` for parsers, rapid for state machines.
- Postgres only through `testing/testutil/pgtest`; integration tests use real Postgres and run in CI.
- Scenarios replay `testdata/scenarios/*.yaml` on the `fake` harness and compare book events with golden files (`-update`, then review the diff).
- Harness contract tests against a real CLI (T8) are opt-in.
