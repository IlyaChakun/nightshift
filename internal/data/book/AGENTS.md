# internal/data/book

The book is the factory's memory in Postgres: tasks, events, jobs, outbox, budgets.

- Migrations in `migrations/` are goose files and expand-only: add tables and columns, never
  drop or rename in the release that stops using them. Existing files are never edited.
- Every new table needs its own `GRANT` to the `nightshift` role in the migration. The
  service has no other rights; `book_backup` and `grafana_ro` get only what they read.
- `book.events` is append-only (a trigger enforces it). Write facts as new events.
- Change state only through `flow.Mover`, which does the compare-and-set on
  (state, version, head_sha) and appends the event in one transaction.
- Queries are in `queries/*.sql`. After a change to them or to a migration run
  `make generate`; `make gen-check` fails on a stale `bookdb/`. Never edit `bookdb/` by hand.
- `make sql-lint` (squawk) checks migrations; its exclusions and their reasons are in
  `.squawk.toml`.
- Tests that need Postgres use `internal/testutil/pgtest` and run in CI.
