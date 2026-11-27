# A failed job keeps its reason on worker shutdown

Plan item G5 of `docs/plan/go-quality/`; source: finding F9 of `docs/research/go-quality-audit.md`.

Run on a real stack: start a Builder job on a synthetic task, then stop the worker while the agent runs (`docker compose stop worker`). Expected: the job ends as `error` in the book, and its events hold a `job.error` with the reason. Also note the worker's stop grace: the result and the reason share one 30 s bound (`runner.deliverTimeout`), and a grace shorter than that can cut them off (G10).

The unit test `TestShutdownKeepsTheReasonOfAFailedJob` covers the logic on a fake jobs API only.

Status: pending
