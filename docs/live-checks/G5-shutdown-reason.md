# A failed job keeps its reason on worker shutdown

Plan item G5 of `docs/plan/go-quality/`; source: finding F9 of `docs/research/go-quality-audit.md`.

Run on a real stack: start a Builder job on a synthetic task, then stop the worker while the agent runs (`docker compose stop worker`). Expected: the job ends as `error` in the book, and its events hold a `job.error` with the reason. The result and the reason share one 30 s bound (`runner.DeliverTimeout`); the worker's `stop_grace_period` of 60 s covers it and the stop of the agent, git and the checkout cleanup before it (G10). Expected also: `docker compose stop worker` returns without a kill (no exit code 137); note how long the stop took.

The unit test `TestShutdownKeepsTheReasonOfAFailedJob` covers the logic on a fake jobs API only.

Status: passed 2026-11-28 on c3ce144 (V3): `docker compose stop worker` took 52 s and exited 0; job 72 ended `failed`/`error` with the reason "runner: harness: claude: context canceled" (`~/nightshift-evals/2026-11-28/V3-G5-stop.txt`).
