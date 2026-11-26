# Hold jobs for a capped credit pool

[Q1a](../plan/phase-5/Q1a-monthly-credit-pools.md) covers factory worker leases. Run a disposable Postgres and the service with two uncapped fake providers. With workers stopped, seed one queued job per provider with its harness and auth in the payload. Stop the service, set the first provider's pool to a numeric ceiling, and restart against the same book. Call the authenticated jobs claim endpoint twice and inspect both database rows.

Status: passed 2026-11-26 on the Mac using the service at 625cccd and PostgreSQL 18.1. With a $3 ceiling, the first job stayed queued at attempt 0. The second job received the first lease at attempt 1. The next claim returned no job. The run did not call a model or exercise funded reservations. Output and reproduction script: `~/nightshift-evals/2026-11-26/Q1a-pool-restart.{txt,py}`.
