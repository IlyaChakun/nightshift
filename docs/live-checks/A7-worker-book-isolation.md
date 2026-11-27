# Worker cannot reach the book or trusted Git

- What. [A7](../plan/architecture/README.md) adds static negative tests: the Postgres server shares no network with the worker and the worker holds no Postgres credential (`deploy/compose_test.go`, `TestWorkerCannotReachTheBook`), and no package the worker roots reach calls the raw Git runner of `gitops` (`internal/testing/archtest/trusted_test.go`). Neither proves what a running container can do.
- Run. On the stable instance: `docker compose exec worker sh -c 'getent hosts postgres; nc -zw3 postgres 5432; echo rc=$?'`, `docker compose exec worker ls /run/secrets /run/worker-secrets`, and `docker compose exec worker sh -c 'git -c safe.directory=* -C /repos/<owner>/<repo>.git push origin HEAD:refs/heads/ns/escape-test'`. Keep the output under `~/nightshift-evals/`.
- Expect. The name `postgres` does not resolve and the connection fails; no `pg_*`, `github_*` or chat secret is listed; the push fails with an authentication or connection error (no token, no route to the code host), not with a `safe.directory` error, which would prove nothing.

Status: pending
