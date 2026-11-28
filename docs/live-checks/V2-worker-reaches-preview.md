# The worker reaches a preview

The worker opens a task's preview through `egress-preview` by its name `<project>.preview.internal` (ADR 0028), and nothing else through that proxy.

On the stable instance with the `deploy` profile, bring up a preview of a test tenant with `compose-local`, then from the worker:

1. `docker compose exec worker sh -c 'curl -sS -o /dev/null -w "%{http_code}\n" "$URL"'` with the URL from the acceptance job's prompt (`http://ns-<tenant>-<task>.preview.internal:<port>`): expect `200`.
2. The same with `http://host.docker.internal:3000` and `http://example.com`: expect a refusal by the proxy.
3. `docker compose logs egress-preview`: one allowed request for the preview name, the others denied.
4. The deployer's `up` succeeds although the mirrors belong to another user than the deployer: no "dubious ownership" in `docker compose logs deployer` (second V2 run, 2026-11-28).
5. A deployer that keeps failing escalates the task after the third failure at one head instead of retrying every tick (`nightshift status --task <id>`: `escalated`, event `preview.failed` three times).
6. The Verifier's acceptance job opens the preview in Chromium through `HTTP_PROXY` (skill `browser-acceptance`): a screenshot of the preview in the job's evidence.
7. `deploy/upgrade.sh --check-only` with that tenant passes on a commit whose sandbox recipe is `compose-local`.

Not covered by the run in CI: Docker Desktop on macOS (DNS for a dotted alias, the subnet free on the Mac), and the agent's own Chromium in the worker (a local Playwright run through a test proxy passed, 2026-11-28).

Result so far: steps 1–4 passed 2026-11-28 on c3ce144 (`~/nightshift-evals/2026-11-28/V2-worker.txt`, `V1-task60.txt`): the worker got `200` from the preview; Chromium as the agent user (uid 20001) with the skill's proxy flags loaded the page and took a screenshot, while `example.com` got `407`; `host.docker.internal` was refused; the `egress-preview` log allows only the preview name; the deployer's `up` worked in the V1 sandbox task. Chromium's own calls to `clients2.google.com` and `update.googleapis.com` were denied by `egress-preview`; the skill turns them off since then. Pending: step 5, step 6 (an acceptance job, in V4) and step 7 as its own run (V1's upgrade covers the `compose-local` recipe).

Status: pending
