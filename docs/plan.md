# Plan

Phases and their "Done" criteria: `docs/design.md`, section 22. This file lists the open work. Limit: 150 lines.
Rules: one item = one PR, under ~400 changed lines; tick it in that PR. Add an item only if it follows from the design and the owner agreed.

Phases 0–4 are closed in code (PRs #1–#190). The live Done
checks of phases 1 (restore, restart) and 2 (no token, no way past the allowlist) passed on a real stack
with `make drill` on 2026-11-25 (`docs/status.md`). Phase 2 item 10 is ticked on
the recommended reading (model-call details come from the logs). Open owner decisions: `docs/decisions.md`.

## Current phase: 5 — Lead: planning and support

Phase 5's work is under "Later phases"; below: the owner's audit follow-up (2026-11-25).

- [x] A1. Honest plan and status, `docs/decisions.md`
- [x] A2. `make drill`: restore, restart, no token, no egress; green on the owner's Mac 2026-11-25 (22 of 22)
- [x] A3–A5. Design-vs-code check (`cmd/nightshift/design_test.go`); rule for unverified external facts (AGENTS.md, Lead role); limits measured by source, usage query in the runbook (decision 10)
- [x] Q0. Promise checks: `cmd/nightshift/promises_test.go` (prompt commands, role skills and keys, schema fields); skill `self-audit`; AGENTS.md "Done means checked"
- [ ] Q1. Wire or remove every schema field in the allowlist of `promises_test.go`, one PR per group (wired: `project.protected`, `tenant.language`); the check matches key names, not paths, so a common word hides a gap; command scripts are found by a simple rule (a program with a directory, or a script extension), computed paths must be listed in `protected`
- [ ] Q2. `playwright/playwright-cli` is vendored or dropped from the Verifier (prompts no longer name `ns search`: `nightshift search` is built and Lead gets hits in its prompt; role keys `inputs`, `fresh_context`, `on` are removed)
- [x] H1. Factory health: a digest part and a task per open alert or idle PR (canceled when gone); alerts need App right `security_events: read`
- [x] R1. A timeout per conductor step: `Ticker.StepTimeout` (30 minutes) cuts a hung step, logs it and leaves the task for the next tick; a canceled git gets SIGTERM before SIGKILL so its lock files go; T1. Open: a wait reason in the book, not only the log (ADR 0018)
- [x] R2. Every conductor and train side effect repeats safely (ADR 0018): `OpenPR` returns the open pull request of the branch (#263); `sideeffects_test.go` lists each call with the reason a repeat is harmless and fails on a new one; the hand-over calls repeat on the twin. Not verified on real GitHub
- [ ] S1. `feat(runner): one supervisor for every harness` (ADR 0019) — stall, repeat, token and dollar rules on `claude` and `codex` events, group stop, limits per role and tenant; T1. Mac re-check 2026-11-26: tokens undercounted, fixed (#305). Open until the Mac checks both stream formats live (T8)
- [ ] S2. `feat(summary): a task timeline` — done: `nightshift status --task <id>` merges `events` and every job of the task (who, when, exit, time, tokens, cost, why it stopped); the Grafana "Task timeline" dashboard reads `book.dash_task_timeline` as `grafana_ro`; T1, T3. Open until the dashboard shows a task on the Mac's obs profile
- [ ] S3. `feat(dispatch): model and effort per role` (ADR 0020) — role, tenant `roles`, provider pin; claude and codex flags; Lead and Verifier pinned at `high`; T1. Open until a subscription run on the Mac

## Phase 3 — Builder, checks, gates (built; open: item 10 review reminders, D2, L4)

Items follow design section 22. Phase 3 also builds a minimal Lead and the Verifier's review leg on `fake` (owner, 2026-11-25); live runs of both wait for the owner.

- [x] 1. `feat(check): parse SARIF and diff findings against a baseline` — fingerprints, new
  violations only, findings by `schemas/finding.json`; T1 and fuzz
- [x] 2. `feat(check): add nightshift check` — runs the project's `verify` and factory tools,
  writes SARIF to the artifacts directory, exits non-zero on new violations
- [x] 3. `feat(roles): load roles from YAML and add Builder` — loader and `roles/builder.yaml`; findings
  returned to Builder, `dispute`
- [x] 4. `feat(policy): reject patches on protected paths` — constitution checked on accept
- [x] 5. `feat(flow): apply transitions atomically` — CAS in the book, events appended
- [x] 6. `feat(github): PRs from nightshift, CI polling, human-review status` — twin first
- [x] 7. `feat(train): merge train with a step journal and restart reconciliation`
- [x] 8. `feat(policy): blast radius, review mode, cancel and /ns pause` — classifier, review decision and
  ladder in #78; the `settings` table, pause in claims and trains, cancel that stops jobs here. The
  `/ns` commands arrive with the Slack wiring
- [x] 9. Morning summary as one message and `/ns status` (done-without-changes share, median time to merge) —
  `internal/summary` builds it from the book; the Slack command and the 09:00 job arrive with the wiring
- [x] 10. GitHub and Linear twins; T4 and T5 scenarios (a third red check escalates). Steps: [x] runner runs
  `verify` as the agent user and sends findings; [x] conductor, building leg (queued → build job → checks
  → patch → PR → CI); [x] conductor, review gate (policy decides human review), hand-off and train; [x] conductor: Verifier leg (read-only job on the pushed head, `dispatch.Decide`, findings back to Builder, a raised blast radius holds for a person; off without a verifier role and provider); [x] Lead stage (chat idea → Lead job → reply or spec card → Approve/Edit/Reject buttons → Builder gets the spec; `lead.go`); [ ] review reminders; [x] Builder jobs carry project instructions and the L0 checklist (`internal/standards`); [x] serve loop that ticks the conductor (`--repos`, off by default); [x] T4 integration scenarios on the GitHub twin (return after red check, third red escalates, request changes, rebase conflict, cancel, human review, hand-off); [x] tracker twin (`internal/adapters/tracker/fake`; a real Linear adapter comes with the first tenant that uses it, phase 8); [x] YAML scenario runner (`TestScenarioHappyPath` replays `happy-path.yaml` and compares the events with a golden file); [x] more scenario files (`spec-rejected`, `changes-requested`; the runner replays every `*.yaml`)
- [x] 11. `tenant init --discover` (reports stacks), `onboard-tenant`, `tenant doctor` (fails without a stack's toolchain), `tenant init --open-pr`
- [x] 12. `feat(runner): assemble role context` — role instructions, `project.yaml: instructions` and
  the L0 checklist in one system file, selected skills in the job directory (design section 12);
  T1 golden on `example-solo`. Without it Builder runs on the task prompt alone
- [x] 13. Done check on real infrastructure: `docker compose build` and `up`, one real task end to
  end. On the `nightshift` tenant a chat idea went through Lead, the owner's approval, Builder,
  CI, review and the merge train to `released` (#148)

- [x] 14–18. Findings and notes in the book; package notes; session handoff; the Lead stage in Slack (`CheckLeadReply`); codex cost. T1, T4 on `fake`.

## Setup automation (owner request; done)

- [x] Z1–Z5. `nightshift doctor` (one line per setup step, `--worker`), `task add`, the GitHub App from a manifest, the chat app from its manifest, `nightshift setup`; T1, T3. Steps are in `docs/setup.md`.

## Dogfooding (owner request, 2026-11-25)

- [x] D1. `feat(scripts): run a stable factory instance` — `scripts/stable.sh` runs a tag as its own
  compose project; there `low` risk merges on green CI, the rest waits for the owner
- [ ] D2. Small plan tasks go through the stable instance one at a time, each independently
  reviewed; factory defects found are fixed in normal PRs
- [ ] D3. Stop hook self-audit (#253 closed; branch `claude/project-thread-itaacy`): reopen after Mac live check

## Other doors besides Slack (owner request, 2026-11-25)

Slack stays the default. A tenant without Slack, or one who works from their own Claude, drives the
same task thread through the book (design section 15, ADR 0006). They start after phase 3 items 18, 16
and 17, one item per PR, in this order. The tenant schema value `inbox` needs the owner's review.

- [x] I1–I3. inbox adapter, CLI `status`, `inbox`, `reply`, `approve`, and `nightshift mcp` over stdio
- [x] I4–I5. remote door over HTTPS (off by default, loopback, short-scope token; T6); `nightshift setup` picks the door
- [ ] I6. `feat(deploy): reach the remote door` — `nightshift` publishes no port, so a tunnel cannot reach its
  loopback listener (`deploy/docker-compose.yml`); add an opt-in `tunnel` profile with the ngrok agent
  (decision 17, `docs/research/remote-door-tunnel.md`); T6: no profile, no listener
- [ ] I7. `feat(mcp): read tools for results` — `task_timeline` (as `nightshift status --task`), `digest`
 , `counters` (24-hour task, token and cost counts behind Grafana); T1
- [ ] I8. Live check on the Mac (decision 7): a claude.ai connector with a static bearer reaches the door through
  the tunnel, Codex connects with `bearer_token_env_var`; then fix the beta line in design section 25

## Limits and the night queue (owner request, 2026-11-25; ADR 0012)

One item per PR, in order; L2 changes `schemas/` and needs the owner's review.

- [x] L1. `fix(dispatch): a limit parks the job, not the task` — limit and reset in the book, no escalation; T1, T3
- [x] L2. `feat(dispatch): wait or fall back by the role chain` — `max_wait`, `tier`, per-task `--allow-fallback`; schemas (owner review)
- [x] L3. `feat(cli): plan tasks ahead` — `not_before`, `priority` on `task add`, honored by the conductor; migration; T3.
  Left for Lead's `to-tickets` (phase 5, dependencies phase 10): the same fields on Lead tasks; `nightshift queue`
- [x] L4. `feat(summary): limits in the morning summary` — limit hits per provider and fallback jobs in `summary.Build`; the 09:00 job that sends it is not wired yet
- [x] L5. `docs(skills): own skill plan-night-queue` — unattended-ready tasks, reading the queue

## Task intake (owner request, 2026-11-26; `docs/research/task-intake.md`)

Phase 5 Lead work; each starts after the phase 5 thread's open PRs on `roles/` and `lead.go` merge.

- [x] N1. `feat(lead): understood line` — `understood` (at most 25 words) in `schemas/lead-reply.json`, shown on question and spec cards; T1, T4.
- [x] N2. `docs(skills): task clarity check in challenge` — goal, done criterion and scope; an empty one means one question with options, not a spec.
- [x] N3. `feat(lead): quote the owner per criterion` — `source_quote` in `schemas/spec.json`; the Verifier gets the owner's message beside the spec.
- [x] N4. `test(evals): Lead intake cases` — frozen cases `several-asks`, `garbled-name` (ask) and `clear-task`, `over-specified` (spec); `expect: ask|spec` is graded by `eval run --role lead`. N7 (done): multi-turn cases (`thread`) grade a correction with `keep` and `drop`.
- [x] N5. `feat(lead): coverage check` — code check that each number, name and negation the owner wrote is in the spec; a miss goes back once (`docs/research/safety-nets.md`); T1.
- [x] N6. `feat(lead): independent reading` — role `reader` sees only the owner's messages and lists what "done" means; what the spec lacks goes on the spec card. Off without a `reader` chain; T1, T4 on the fake harness. Switch it on only if planted-misreading cases show a benefit on a real run (not measured).
- [x] N8. `feat(lead): problem reports` — skill `support`, spec field `report`, buttons Create task and Not needed, two eval cases. Open: a live reproduction.
- [ ] L5. `feat(summary): Lead run record` — provider, model and effort per Lead run in the job result and in the S2 timeline; an effort pin per role in `providers.yaml` (the owner picks the value).

## Several tenants and agent-made configuration (owner request, 2026-11-25; ADR 0015)

One instance builds several projects at once; an agent configures a tenant through files and a PR. One item per PR, in order. The phase 5 thread owns `roles/` and `lead.go`; none of these edit them.

- [x] C1. `feat(cli): nightshift config validate` — instance, providers, tenants and (`--repo-dir`) `.nightshift/` files against `schemas/`, plus references (providers, skills, packs, repos); no secrets or network; T1, T3
- [x] C2. `docs(skills): own skill configure-tenant` — stack, skills, Lead instructions, gates, review mode via files and a PR, with C1 and `tenant doctor`; never secrets, `deploy/` or egress; runbook updated
- [x] M1. `feat(dispatch): capacity.tasks per tenant` — a queued task over the cap stays `queued`; a freed slot goes to the first queued task; T1, T3
- [x] M2. `feat(book): claim fairly across tenants` — oldest job of the tenant with the fewest running jobs per `budget.weight`, a provider's `concurrency` enforced at the claim under a lock, expired leases swept there (#250); one conductor loop per tenant, one tenant per repository; T2, T3.
- [ ] M3. `feat(runner): isolate worker state between tenants` — done: no checkout outlives its job, planted links not followed; T6. a HOME per tenant (`/home/agent/tenants/<tenant>`). Open: mise data the agent cannot write, `/tmp` and the rest of the work root, replicas sharing `/work`, own mirror only
- [x] M4. `feat(config): code-host credential per tenant` — `deploy/secrets/tenants/<name>/github_token`, instance fallback; `docs/secrets.md`; T1
- [x] M5. `docs(runbook): several projects on one laptop` — add a tenant (restart); raise `claude-sub` `concurrency` in `providers.yaml` within `weekly_share`, and a check that the sum of tenants' demand fits it. Later: reload without restart, a per-tenant provider share, MCP write tools through PRs only

## Phase 3 done when

See design section 22; the scenarios and gates are green in CI, item 13 ran on real infrastructure (#148).

## Later phases (design section 22 has the full lists)

Each starts only when the previous Done criterion is met. Pieces already built ahead of their phase
are noted; they are not repeated in the phase's open work.

- **4. Standards and the document quality loop.** Built: `go`, `python`, `ts` stack packs; their
  standards cards; doc-lint in `nightshift check` (ADR and PRD rules, `check.DefaultDocKinds`); discovery reads stacks from the packs' `detect` and names languages without a pack; a job with nothing to check says so (`check.nothing_to_run`). `standards list` and `standards sync` (base Go and Python configs from `standards/<set>/<stack>/files/`, never over a project's own file; the PR step comes with `tenant init`). Also built (ADR 0009, 0010): `mise install` and `project.yaml: setup` before the agent, run by the `toolchain` service with its own egress; Debian worker image; `tenant doctor` for a pinned toolchain; packs `js`, `java-maven`, `java-gradle`; own skills `research-options`, `build-vs-buy`, `web-dev`, `mobile-dev` (selected by `applies_to.deps`); `nightshift deps check`; `nightshift skills sync` (vendors `skills/vendor.yaml` by SHA, scans, writes a diff report; the starter list stays empty until the owner picks); L4 rubric cards (`standards/industry/docs/rubrics/`, `internal/rubric`, `check.RubricFindings` on a `Judge`; the live judge and the Verifier wiring are open). Open: project packs in `.nightshift/stacks/`, standards library, Vale and lychee, templates (PRD, ADR, design), the polygon
  and `nightshift eval`, the L2 research-record finding. Owner (2026-11-25): also stack packs `java-maven`, `java-gradle` and `js` (`rust` with the first Rust tenant), and skills `web-dev` and `mobile-dev` selected by dependencies (mobile means React Native, JS-level checks on Linux CI; native Android and iOS builds and skills, simulators and device e2e are "Later"). (Mutation tests are in design "Later"
  while an older plan line put them here: the owner picks.) Done: a synthetic Rust project passes `tenant doctor` and `verify` without a pack; a planted weak PRD gets L1 and L4 findings; planted code defects are caught by L1 or L4;
  the polygon runs by `evals/selection.yaml`. Polygon (owner 2026-11-25, ADR 0011): runs locally, a throwaway git repository per task, no GitHub; `eval run` built, 1 synthetic task. Closed 2026-11-25 (Done met); `skills sync` and the harness judge (`check --standards --judge-auth`) are in. Leftovers: Vale/lychee (`mise.lock` PR), the standards book index, the 10-task polygon set. Gates (ADR 0014, owner request 2026-11-25), open: `schemas/gates.json` and `project.yaml: gates`; the loader and `nightshift check --plan`; `block`, `warn` and `off` in the check loop with the floor; weakening is `critical` in the classifier; `/ns skip`; T1 and a T4 scenario with one `warn` check. MCP tools to read and, through a PR, change gates, blast radius and review mode (owner request, 2026-11-25).
- **5. Lead: planning and support.** Built: `roles/lead.yaml` stub, reply check, the minimal loop (item
  18). Built: skills `challenge`, `grilling`, `to-spec`, `to-tickets`, `hidden-scenarios` (own; vendor `grilling` later), Builder's `needs_research` rule in its prompt, which sends the task back to Lead (twice at most). Task dependencies built: an approved spec becomes tasks with `book.task_deps`, a task waits in `queued` until its dependencies are `released`, a cancelled one blocks it (the owner's go-on decision button is open). Project memory page storage built (design section 13, ADR 0005): `book.memory_pages` with `revision`, 40 entries and 4 KB cap, `nightshift memory show|remember`; Lead's `memory[]` is accepted only with a quote of an owner message or a repository file as source. Morning digest built (ADR 0013): `nightshift digest` lists merges and decisions with reasons and revert links; the 09:00 post to the owner is built (Slack adapter; `/ns status` still open). The prune also drops an entry whose text `AGENTS.md`, `CLAUDE.md`, `project.yaml` or its cited file now states (reason `in repo: <file>`). Role overrides (`.nightshift/roles/<name>.yaml`, instructions and skills only) built. `/ns status|memory|remember` in chat are built. Also built (2026-11-26, on `fake` and local Postgres only): the book search and duplicate answer (`nightshift search`, `/ns search`, `duplicate_of`), hidden scenarios storage (`lead_verifier` visibility), the doc-lint loop for the PRD and ADR drafts of a spec, and the tracker projection (`internal/dispatch/tracker.go`, twin only). Open: #support, voice, M3 part 2, Q1, Q2 (none is needed for the Done criterion). Done: raw idea → spec →
  tasks without a terminal; 3 duplicates get a link; a fact given once is not asked again.
- **6. Verifier, preview, production.** Built: Verifier role, verdict and the review leg on `fake`; P1 (`integration.Deployer`, the `deploy/fake` twin, `compose-local` in `capabilities` of `schemas/project.json`); P2 steps `up`, `smoke`, `release`, `rollback` through `book.deploy_steps` (`internal/deploy`), the preview gate in the conductor, release through the train with the automatic rollback, on the twin (the full T4 scenario file is open). Open, in order (design section 17 "Deploy targets", ADR 0014): P3 adapters `script` and `external`; P4 built on stubs, its T6 escapes on a real engine open; P5 live acceptance through playwright-cli on the preview, hidden scenarios, #deployments card; P6 revert PR and frozen train on rollback, preview limits and teardown, the polygon run in `deploy/upgrade.sh` (its live check and tag switch are built). `compose-ssh`, `kubernetes` and `load` wait for their first tenant.
  Done: on `compose-local` a Slack task ends with an evidence card and a torn-down preview; Merge deploys; a failed smoke rolls back on its own. Latent: after `revert_merged` the old merged `pr_number` would end the task at once (outside-merge watch); clear it at the `rolled_back` to `building` move.
- **7. Self-improvement.** Built: `internal/learn` clustering. Open (ADR 0016, design section 10 items 10–12): friction counters as `process` findings, the weekly per-tenant what-to-improve report, `nightshift report --preview` with `report` settings in the schemas (owner review), transcript sampling last; the model half of nightly learn and
  its PRs, E2/E3 evals, taxonomy, autonomy ladder, Home tab. Done: a planted recurring defect becomes a
  tool rule overnight; a skill that degrades the polygon is rejected.
- **8. The `team` profile.** Starts with the first team tenant: `handed_off` path, second instance,
  Jira and Confluence, `copilot`. Done: `example-team` scenarios green; two instances isolated (T6).
- **9. Hybrid search on.** Built: nullable embedding column, RRF code. Open: an embedder, backfill.
  Done: hybrid beats full text on the 20-question search eval.
- **10. New projects, multi-repository tasks, dark mode.** Done: a new project connects in under an
  hour; a two-repository spec yields linked PRs; a week of `low` and `medium` tasks without clicks.
