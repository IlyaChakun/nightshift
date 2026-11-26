# Measure verification runtime

State: open

Measure the full `make verify` on one commit with an empty Go build cache and then with the same cache warmed. Record the machine, tool versions, elapsed time and result of each target. Repeat slow targets before choosing an optimisation.

On an Apple Silicon Mac, a baseline at `36af36c` ended at `docker-lint` in 10.99 seconds with an empty Go build cache and 7.21 seconds on the repeat. The pinned linter crashed and the Docker socket was inaccessible from the test shell. These are times to failure. The Go suite did not run.

If database startup dominates the Go suite, compare the standard Testcontainers path with the existing `NIGHTSHIFT_TEST_PG` path backed by a fresh, dedicated test Postgres server. Use the same `make test` suite and include server startup in the comparison. Verify that both runs execute the real Postgres scenarios.

Done when both full verification runs have timings by target and the measurements identify the first optimisation worth testing. Record the before and after timings for any change separately.
