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
