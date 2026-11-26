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
  Renaming that flow event makes every merge count as clean.
- `ProviderJobsSince` counts only jobs with `exit_reason` `done` or `rate_limit`. Fallback providers
  are passed in by the caller; this package does not read config.
- `Format` divides by `Merged`; none says so.
- Every message: head first (`*Title* · window`, counts, merge numbers), then blocks joined by `sep`
  (`---`, a divider in Slack). Lists are capped (`maxNeedsYou`, `maxMergeLines`, `maxDecisionLines`),
  the rest is a count, and `capText` cuts at `maxDigestRunes`. A new block is a capped list.
- `Build` and `BuildDigest` take `until` from the caller's clock, so the head can say "last 24 h".
- "Code without a recorded review" is any merge whose title does not start with `docs` and whose
  description lacks "Reviewed by". `Digest.Factory` (task PRs) only feeds a count.
- Postgres tests use `pgtest`.
- `Timeline` gives one task's history for `nightshift status --task`; `reason` reads a failed job's
  `stopped` or `error` key; a new key goes there.
