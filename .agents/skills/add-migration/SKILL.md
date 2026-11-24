---
name: add-migration
description: Add or change a Postgres table, column or query in the book (internal/book). Use when a task needs new state in the database, a new sqlc query, or a GRANT for a role.
---

# Add a migration

The book is the factory's memory. Read `internal/book/AGENTS.md` first; this skill is the order of work.

1. **Write a new goose file** `internal/book/migrations/NNNNN_<topic>.sql` (next number, `-- +goose Up` only). Never edit an existing migration and never write a Down section.
2. **Expand only.** Add tables and columns. Do not drop, rename or tighten a column in the release that stops using it.
3. **Grant in the same file.** A new table needs its own `GRANT` to the `nightshift` role. `book_backup` and `grafana_ro` get only what they read. A missing grant passes every local unit test and fails in CI with `permission denied for table ...` (the merge train journal hit this).
4. **Write the query** in `internal/book/queries/*.sql`, then run `make generate`. Never edit `internal/book/bookdb/` by hand; `make gen-check` fails on a stale copy.
5. **Change state only through `flow.Mover`.** `book.events` is append-only: record facts as new events.
6. **Lint.** `make sql-lint` runs squawk. Do not add an exclusion to `.squawk.toml` without a written reason there.
7. **Test.** A Postgres test uses `internal/testutil/pgtest` (T3). It needs Docker and runs only in CI, so read the first CI log of the PR. A pure query or parser change gets a T1 test too.

Done when `make verify` is green and the PR names the migration, its grants and the rollback (expand-only: ship a new migration, never undo).
