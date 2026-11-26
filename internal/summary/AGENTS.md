# internal/summary

Builds the morning summary and the `/ns status` answer for one tenant from the book
(design sections 8 and 15). It is plain SQL and text; no model writes it.

- `Build` reads, `Format` renders. Keep the numbers in SQL and the wording in `Format`.
- The queries are in `internal/book/queries/summary.sql`; after a change run `make generate`.
- Only `Finished`, `LimitHits` and `FallbackJobs` depend on the `since` window. `Active`,
  `Waiting`, `Merged`, `WithoutChanges` and `Median` cover all tasks of the tenant.
- State groups are Go slices (`waitingStates`, `activeStates`). A new `flow.State` is counted
  nowhere until you add it to one of them; terminal states are in neither on purpose.
- `Finished` counts tasks with a `task.moved` event to `released`, `merged_by_team` or
  `handed_off`. `MergeStats` leaves out `handed_off`. A new end state needs both queries.
- "Without your changes" means no `task.moved` event with `payload->>'event' =
  'request_changes'`. Renaming that flow event silently makes every merge count as clean.
- `ProviderJobsSince` counts only jobs with `exit_reason` `done` or `rate_limit`. A job on a
  fallback provider that hit its limit counts as a limit hit, not as fallback work.
- Fallback providers are passed in by the caller (`tier: fallback` in the providers config);
  this package does not read config.
- With no merged task the median is 0 and `Format` prints a "no merge numbers" line, so
  `Merged` is the only divisor guard (`Format` divides by it).
- Tests: `summary_test.go`; the ones that need Postgres use `internal/testutil/pgtest` and
  run in CI.
- `Timeline` gives one task's history for `nightshift status --task`: events plus every job
  (`AllTaskJobs`). `reason` reads a failed job's `stopped` or `error` key; a new key goes there.
- `BuildHealth` adds the repository health part to the digest: open alerts, pull requests idle for `IdlePR`, failing checks on `main`. A part that cannot be read is "unknown", never an empty list; the alerts need the `security_events: read` right.
