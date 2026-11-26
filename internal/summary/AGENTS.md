# internal/summary

Builds the morning summary, the digest and the `/ns status` answer for one tenant from the book
(design sections 8 and 15). It is plain SQL and text; no model writes it.

- `Build` reads, `Format` renders. Keep the numbers in SQL and the wording in `Format`.
- The queries are in `internal/book/queries/summary.sql`; after a change run `make generate`.
- Only `Finished`, `LimitHits` and `FallbackJobs` depend on the `since` window. The rest covers all tasks.
- State groups are Go slices (`waitingStates`, `activeStates`). A new `flow.State` is counted
  nowhere until you add it to one; terminal states are in neither on purpose.
- `Finished` counts `task.moved` events to `released`, `merged_by_team` or `handed_off`; `MergeStats`
  leaves out `handed_off`. A new end state needs both queries.
- "Without your changes" means no `task.moved` event with `payload->>'event' = 'request_changes'`.
  Renaming that event makes every merge clean.
- `ProviderJobsSince` counts only jobs with `exit_reason` `done` or `rate_limit`. Fallback providers
  are passed in by the caller; not read from config.
- `Format` divides by `Merged`.
- Message: head first, blocks joined by `sep` (`---`, a Slack divider): needs you, what changed
  (`work.go`: new, fixed, reverted by title type), plan, merges to look at, decisions, health (only
  when wrong), conclusion last. Each fact once. Lists are capped; over `maxDigestRunes` decisions go first.
- `Build` takes `until` from the caller, in the tenant's zone. `BuildPlan` errors add nothing.
- "Unreviewed": title not `docs…`, no "Reviewed by"; listed first, then protected, then `Digest.Factory`;
  `needsYou` drops a task replaced by a later one of the same title.
- Postgres tests use `pgtest`.
- `Timeline` gives one task's history for `nightshift status --task`; `reason` reads a failed job's
  `stopped` or `error` key; a new key goes there. It shows the asked `model`
  and `effort` and `usage.model`.
