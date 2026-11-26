# Apply a Verifier result once

State: open

Source: [audit #334](https://github.com/IlyaChakun/nightshift/pull/334). Commit risk, job consumption and the task transition together. Preserve the first storage error. Count only applied returns to Builder. [A06a](../phase-5/A06a-single-verifier-finding-writer.md) records findings at job completion.

Postgres tests cover simultaneous application, interrupted writes and replay. The tests cover an earlier pass and a stale result from a different revision. The [live check](../../live-checks/A03b-atomic-verdict.md) is pending.
