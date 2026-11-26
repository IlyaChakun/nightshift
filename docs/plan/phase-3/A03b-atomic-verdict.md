# Apply a Verifier result once

State: proposed

Source: [audit #334](https://github.com/IlyaChakun/nightshift/pull/334). Commit findings, risk, job consumption and the task transition together. Preserve the first storage error. Count returns without spending the budget for an earlier pass.

Check simultaneous application, interrupted writes, retries, and an earlier pass on Postgres.
