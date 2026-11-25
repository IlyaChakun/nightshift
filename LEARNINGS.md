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

## First live runs

- Smokescreen has no flag for `allow_missing_role`; it lives in `deploy/egress-config.yaml`.
  Without client certificates every request has no role, and without that setting the proxy
  refuses all of them.
- An agent CLI needs `HOME` and the proxy variables (`HTTPS_PROXY`, `NO_PROXY`) in its
  environment; the worker passes them on purpose. A run that works by hand and hangs in a job
  is usually missing one.
- Both model CLIs rejected a role result schema with `$schema`, `$id` or references to sibling
  files. `schemas.Standalone` inlines them; results are still validated against the original.
- A fine-grained token cannot read check runs; read CI from Actions runs and commit statuses.
- Never put a build output in the tree (a 37 MB binary once got in); `repo-check` fails on files
  over 1 MiB.

## CI history (13 red runs in phases 2-3)

- Every red run had one of five causes: a lint rule (gosec G115 on `string(rune(n))`, misspell
  `cancelled`, nilnil), a missing `GRANT` on a new table, a handoff file over its line limit,
  a design section changed without the page, a parallel stub-binary test (`text file busy`).
  Read the table in `.agents/skills/fix-ci/SKILL.md` before pushing.
- Docs-only PRs break `repo-check` too (status.md at 41 lines, design-sync). Run `make repo-check`.
- Postgres tests run only in CI; the first CI log is the real test result for a migration.
- A third of recent runs were cancelled by a newer push; wait for the last run instead of
  reading a cancelled one.

## Slow CI

- Twice the `test` job sat at about 120 s (run p50 129 s, queue 2 s) because every integration
  test started its own Postgres container and ran all migrations, 4 at a time under `-race`
  (`dispatch` 75 s, `book` 31 s). Fix: `pgtest.Start` keeps one container per test binary,
  migrates a template database once and gives each test `CREATE DATABASE ... TEMPLATE`
  (`dispatch` 13 s, run p50 about 75 s). Role changes are cluster-wide, so they happen once in
  the shared setup; parallel `ALTER ROLE` fails with "tuple concurrently updated".
- `repo-check` fails on a Postgres container started outside `pgtest`. Measure before fixing:
  see "CI is slow" in `.agents/skills/fix-ci/SKILL.md`.
