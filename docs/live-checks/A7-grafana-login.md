# Grafana asks for a login

- What. [A7](../plan/architecture/README.md) turns anonymous access off in the obs profile's Grafana and gives the `admin` user the generated secret `grafana_admin_password`; `deploy/grafana/start.sh` sets it on an existing data volume too (`deploy/compose_test.go`, `TestGrafanaNeedsALogin`). Checked on 2026-11-27 with the pinned `grafana/otel-lgtm` image under plain `docker run`, not under compose.
- Run. On the stable instance after `scripts/stable.sh up <tag>`: `curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/api/search`, the same with `-u admin:admin`, and with `-u "admin:$(sudo cat deploy/secrets/grafana_admin_password)"` (on macOS without `sudo`); then from the worker, `scripts/stable.sh compose exec worker sh -c 'curl -s -o /dev/null -w "%{http_code}\n" http://lgtm:3000/api/datasources'`. Keep the output under `~/nightshift-evals/`.
- Expect. 401, 401, 200, and 401 from the worker; the Task timeline dashboard still loads after the login.

Status: passed 2026-11-28 on a4dc6b8 (`factory-stable-2026-11-28`): 401 anonymous, 401 with `admin:admin`, 200 with the generated password, 401 from the worker (`~/nightshift-evals/2026-11-28/V1-A7-grafana.txt`).
