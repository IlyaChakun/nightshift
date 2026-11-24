# LEARNINGS

Lessons that no tool, test or schema could encode yet (design section 10, fix ladder rung 5).
Keep it under 100 lines. Before adding a lesson, look for a similar one and update it. When a
lesson becomes a tool rule, delete it and link the rule. Changes need the owner's Approve:
this file is a protected path.

## Go and CI

- The Go toolchain in an agent session can be older than the one in `mise.toml`, and the
  linter then cannot run locally. Run `make tools` first; if the pin differs, expect lint
  errors to appear only in CI and read the job log instead of guessing.
- `fmt.Fprintf` into a hash trips errcheck. Build the string and hash it.
- Tests that need Postgres use testcontainers and run only in CI. A session without a Docker
  daemon cannot run them, so keep them small and read the CI log of the first run.

## Book and SQL

- After a change to `internal/book/migrations` or `queries/`, run `sqlc generate`. `sqlc diff`
  fails CI on a stale `bookdb/`.
- Migrations are expand-only and have no Down section. `.squawk.toml` lists the rules that do
  not apply, with reasons; do not add more without one.

## Agents and notes

- A note an agent proposes is unverified. It never goes into a prompt on its own; learn may
  promote it here, and then it is a lesson with a source.

## CI history (13 red runs in phases 2-3)

- Every red run had one of five causes: a lint rule (gosec G115 on `string(rune(n))`, misspell
  `cancelled`, nilnil), a missing `GRANT` on a new table, a handoff file over its line limit,
  a design section changed without the page, a parallel stub-binary test (`text file busy`).
  Read the table in `.agents/skills/fix-ci/SKILL.md` before pushing.
- Docs-only PRs break `repo-check` too (status.md at 41 lines, design-sync). Run `make repo-check`.
- Postgres tests run only in CI; the first CI log is the real test result for a migration.
- A third of recent runs were cancelled by a newer push; wait for the last run instead of
  reading a cancelled one.

