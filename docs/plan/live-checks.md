# Live checks — merged work that has not run on a real stack

The queue of everything merged on fakes, twins or stubs whose check on a real stack, harness or service is still owed. A feature here is "merged, live check pending", not done. The live-check session on the owner's Mac takes entries from the top; a thread that merges such a feature adds its entry in the same PR (AGENTS.md, "Current stage"). Audit fixes have [individual live checks](../live-checks/). Rules and limits: `docs/plan.md`.

## How to use

- **Add** an entry in the PR that merges the feature: what, PR, what to run, what to expect. Append it; do not reorder.
- **Run** on a real stack, never on a twin. Keep result files outside the repository (`~/nightshift-evals/`).
- **Record** on the entry's `Status` line: `passed <date>, <evidence>` or `failed <date>, <evidence>, fix: <plan item or PR>`. Evidence is a task id, a commit, a log line or a file path that someone else can open. Without evidence the entry stays `pending`.
- A `failed` entry gets a plan item in the phase file of its feature and stays here until the re-run passes.
- A `passed` entry moves to "Passed" as one line. Edit only your own entry, so parallel sessions do not conflict.

## Pending

### Deployer on a real Docker engine

- What: recipe lint as an allowlist (#330), a release that never turns healthy rolls back on its own (#353); phase 6 P4.
- Run: `docker compose --profile deploy up -d --build`; then the escape recipes of the first live run, one release whose stack stays unhealthy, and one plain task to a preview on 127.0.0.1.
- Expect: every escape recipe is refused before any container starts; the unhealthy release ends `rolled_back` without a person; the plain task leaves the preview gate with an evidence card.
- Status: pending

### Deployer refuses a recipe that reads another file

- What: the recipe is one file; `include`, `extends` of another file, `label_file`, a key with a `!!binary` or `!!merge` tag, a recipe in a linked directory and a `.env` that is a link out of the checkout are refused before the render (`internal/adapters/deploy/composelocal/source.go`; phase 6 P4).
- Run: with the `deploy` profile's deployer image built from `main`, the recipe of the 2026-11-26 re-test that reached a file outside the checkout through a link, once with `extends` and once with `include`; then a checkout whose `.env` is such a link; then the plain sandbox recipe.
- Expect: each of the three is refused with "another compose file" or "file outside the checkout" and `docker compose config` never runs for it; the plain recipe still comes up.
- Status: pending

### `deploy/upgrade.sh` accepts one-shot services

- What: a service that exited with 0 is not a failed upgrade (#326, reads `.ExitCode`).
- Run: `deploy/upgrade.sh` on the stable instance, with its one-shot services present.
- Expect: the upgrade ends green; a service that exited non-zero still fails it.
- Status: pending

### Worker image size

- What: Chromium and `playwright-core` in the worker image (#329; phase 5 Q2).
- Run: `docker build --target worker -t ns-worker-check .`, then `docker image ls ns-worker-check`.
- Expect: the measured growth is at most about 1 GB over the previous worker image (owner's limit, 2026-11-26); record size and build time.
- Status: pending

### Chromium starts as uid 20001

- What: browser acceptance runs under the worker's own restrictions (#329; phase 5 Q2).
- Run: the image above with `--user 20001 --cap-drop ALL --security-opt no-new-privileges`, a script that launches `/usr/bin/chromium` through `playwright-core` and writes a screenshot to `/tmp`.
- Expect: the browser starts, prints its version and writes the screenshot; otherwise the exact error.
- Status: pending

### Quiet hours hold a release

- What: `schedule.quiet_hours` holds owner cards and production releases (#325).
- Run: set a window that contains now on a tenant whose repository has a release target; take a small task to the merge step; then move the window away. Repeat on a repository with no release target.
- Expect: no merge inside the window and the log line "release waits for the end of quiet hours"; the next tick after the window merges; without a release target it merges at once.
- Status: pending

### Review reminder

- What: `review.remind_after_hours` (#331).
- Run: set `[1]` on a test tenant and leave one pull request waiting.
- Expect: one reminder card after about an hour; inside quiet hours it arrives when the window ends.
- Status: pending

### SLA auto-merge

- What: `review.sla_automerge_hours`, off unless the tenant sets it (#331).
- Run: on a test tenant only, set it to 1 and leave one green pull request waiting; leave a second tenant without the key.
- Expect: the first merges itself once after the hour with its gates green; the second never does.
- Status: pending

### Morning digest in Slack

- What: the digest that leads with results: a head, "What changed", "Plan", "Merged" with protected paths and unreviewed code listed, a conclusion that repeats nothing, times in the tenant's zone (#316, #324, #360, #365).
- Run: `nightshift digest <tenant> --send` on the stable instance, after the upgrade to a commit that holds #365.
- Expect: Slack renders the sections and dividers, with no raw markup and no cut message; times carry the tenant's zone; each plan line is the heading of a plan file with open items; when the conclusion counts unreviewed code or protected paths, "Merged" lists some of each; "Nothing waits for you" appears once; the links open the pull requests and the two searches.
- Status: pending

### Alert counts in the digest

- What: open code-scanning alerts and idle pull requests in "Factory health", and tasks made from them (#319, #328).
- Run: the digest above, beside `gh api repos/<owner>/<repo>/code-scanning/alerts` for the same repository.
- Expect: the counts are equal; no "code-scanning alerts: unknown" line; one task per open alert, none twice.
- Status: pending

### Lead card labels in the chat language

- What: card buttons and texts follow the chat language; the spec card has kind `spec_for_approval` (#359; phase 5 C1).
- Run: one idea written in Russian in Slack, through a real Lead, to the spec card.
- Expect: every label on the question and spec cards is Russian and Slack renders them whole.
- Status: pending

### Approve hands the spec to Builder

- What: the Approve button on the spec card turns the spec into tasks and starts Builder (phase 3 item 18, #359).
- Run: press Approve on the spec card of the check above.
- Expect: the reply "Approved. Builder starts now." in the chat language, tasks in the book, a Builder job started; the task does not stay at the card.
- Status: pending

### Tracker projection

- What: tasks projected into the tenant's tracker, cancel and priority accepted back (#284; twin only).
- Run: a tenant with a real tracker; approve a spec, then cancel one task and raise another's priority in the tracker.
- Expect: one tracker item per task, no duplicate on a retry; both changes reach the book. Needs a tracker account from the owner.
- Status: pending

### Independent reader chain

- What: role `reader` lists what "done" means from the owner's words alone; gaps go on the spec card (#304; phase 5 N6).
- Run: a tenant with a `reader` chain and the planted-misreading eval cases on a real harness.
- Expect: the spec card names what the spec lacks; the cases show whether the reader catches more than Lead alone (decides if it is switched on).
- Status: pending

### Supervisor on `codex`

- What: stall, repeat and token rules on `codex` events (ADR 0019; phase 5 S1, T8).
- Run: one `codex` Builder job with a low token limit.
- Expect: the run stops mid-way and the timeline names the reason.
- Status: pending

### Problem report reproduction

- What: skill `support`, buttons Create task and Not needed (#317; phase 5 N8).
- Run: one problem report in the support channel of a test tenant.
- Expect: a reproduction finding on the card; Create task makes one task.
- Status: pending

### Side effects repeat safely on real GitHub

- What: `OpenPR` returns the open pull request of the branch; hand-over calls repeat (#263, #264; phase 5 R2).
- Run: stop `nightshift` between push and pull request on a test repository, then start it.
- Expect: one pull request, one comment of each kind, no second push error.
- Status: pending

### Several tenants

- What: two tenants at once, the tenants bind mount, the one-click GitHub App creation (`docs/plan/tenants/README.md`).
- Run: `nightshift setup` for a second tenant on the Mac, then one task in each at the same time.
- Expect: both reach a pull request; neither job sees the other's checkout or HOME.
- Status: pending

### Worker death sweeps leases

- What: a job whose worker died is leased again (`docs/status.md`).
- Run: `docker compose kill worker` during a Builder job, then start it.
- Expect: the job runs again once and the task ends; no task stays leased.
- Status: pending

### Remote MCP door

- What: a claude.ai connector reaches the door through the tunnel (`docs/plan/doors/README.md` I8, decision 7).
- Run: after I6 and I7, as I8 describes.
- Expect: the connector lists tasks with a static bearer token; no listener without the profile.
- Status: pending (waits for I6)

### Lead uses the book search and the project memory

- What: `roles/lead.md` names the two prompt parts it is given instead of calling them unavailable (#364).
- Run: on a real Lead, one idea in chat that repeats an earlier task of the tenant; then a question whose answer is a fact on the tenant's memory page (`nightshift memory show <tenant>`). The frozen eval cases do not show this: their prompt carries neither part.
- Expect: the repeated idea gets the earlier task as `duplicate_of` and no spec; the fact from the memory page is used and not asked again.
- Status: pending

### `make docker-lint` on an arm64 Mac

- What: `mise.lock` maps macOS arm64 to the x86_64 `hadolint` binary, which crashes there, so `make docker-lint` falls back to the official image of the same version when the local binary does not start. Checked on Linux only, with the binary absent.
- Run: in a shell without `mise activate`, `make tools` (the Makefile puts the mise tools on `PATH`), then `make docker-lint`, then `make verify` to the end.
- Expect: `hadolint --version` fails there; `make docker-lint` prints "using hadolint/hadolint:v" and the pin from `mise.toml`, exit 0; `docker-lint` and the rest of `make verify` finish. The `gitleaks dir` scan of the ignored `.env` may still fail (`docs/status.md`).
- Status: pending

### Plan folders in the digest

- What: the digest's plan line reads the `README.md` of each folder under `docs/plan/` (#368).
- Run: `nightshift digest <tenant> --send` on the stable instance, after the upgrade to a commit that holds #368.
- Expect: the plan line names the open parts with the item counts of their files, for one `docs/plan/phase-5/README.md` and `docs/plan/phase-3/README.md`; no item file of a folder is counted.
- Status: pending

### Pre-#310 state leaves the shared agent HOME

- What: after the release is up, `deploy/upgrade.sh` moves `.claude`, `.claude.json` and `.codex` from `/home/agent` into `/home/agent/.pre-tenant-state/<time>` as root (`deploy/legacy-home.sh`, phase 5 M5).
- Run: `deploy/upgrade.sh` on the stable instance; then `scripts/stable.sh compose exec worker ls -la /home/agent` and `scripts/stable.sh compose exec -u 20001 worker ls /home/agent/.pre-tenant-state`.
- Expect: the upgrade prints `legacy home state moved: ...`; the three names are gone from `/home/agent`; `.pre-tenant-state` is owned by root with mode 0700; uid 20001 gets "Permission denied". A refused `--cap-add` is a failure to fix, not to skip.
- Status: pending

### Codex on the owner's ChatGPT plan

- What: provider `codex-sub` (`auth: chatgpt`) runs Builder with a personal access token and no API key (phase 5 S4); also the codex part of S1.
- Run: put the token in `deploy/secrets/harness/CODEX_ACCESS_TOKEN`, add `codex-sub` to `providers.yaml` and as the tenant's first Builder provider, rebuild the worker with `CODEX_VERSION` set, then one sandbox task of `example-solo`; `nightshift status --task <id>`.
- Expect: the job ran on `codex-sub`, usage tokens are recorded, the task reaches `released`; the egress log shows only `chatgpt.com` and `auth.openai.com` for OpenAI; no `OPENAI_API_KEY` or `CODEX_API_KEY` is in the job's environment. Record where ChatGPT creates the token and fix `docs/runbook.md` with it.
- Status: pending

## Passed

| Check | Result | Evidence |
|---|---|---|
| Supervisor on `claude` (S1) | passed 2026-11-26; one long model message overshoots the limit | task #44, `docs/plan/phase-5/README.md` S1 |
| Task timeline (S2) | passed 2026-11-26 | task 1004 on 606ae8b, `docs/plan/phase-5/README.md` S2 |
| Model and effort per role (S3) | passed 2026-11-26 | task #42, `docs/plan/phase-5/README.md` S3 |
| HOME per tenant (#310) | passed 2026-11-26; isolation is by path only (M4 open) | `docs/plan/phase-5/README.md` M4 |
| Real Lead on the intake cases | passed 2026-11-26, 5 of 5 by the Mac session's account | result file not recorded |
| Restore, restart, no token, no egress | passed 2026-11-25, 22 of 22 | `make drill`, `docs/status.md` |
