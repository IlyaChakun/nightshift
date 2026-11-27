# Keep each attempt of a job to its own lease, checkout and patch

State: open

Source: [audit #335](https://github.com/IlyaChakun/nightshift/pull/335), items A04a, A04b and A04c; part of M3. An attempt stops when its lease is lost and starts no turn after it. Checkouts and patches are named by attempt, and the worker removes a crashed run's checkouts when it starts. See [ADR 0025](../../adr/0025-attempt-scoped-worker-state.md).

Check with `go test ./internal/runner ./internal/jobsapi ./internal/jobserver -run 'Lease|Attempt|Turn|Leftover'` and `go test ./internal/dispatch -run 'TestAStaleAttemptCannotReplaceTheCompletedPatch|TestIsPatchPath'`. The [live check](../../live-checks/A04-attempt-isolation.md) is pending.
