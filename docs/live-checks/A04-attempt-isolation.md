# Attempt-scoped worker state

- What. [A04](../plan/tenants/A04-attempt-isolation.md) stops an attempt whose lease is lost and gives each attempt its own checkout and patch directory. Source: [audit #335](https://github.com/IlyaChakun/nightshift/pull/335).
- Run. On the stable instance, start a Builder job and stop the `nightshift` service for longer than one lease while the turn runs, then start it again. Watch the worker log and `/work`, then let the requeued job finish.
- Expect. The worker logs the lost lease and stops the turn. The requeued attempt uses a new `/work/<tenant>/<task>-<attempt>` directory, its patch is under `/artifacts/<job>/<attempt>/`, and the task advances with that patch. After a worker restart no old checkout remains in `/work`.
Status: pending
