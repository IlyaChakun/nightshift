# Phase 5 — Lead: planning and support

Owned by the session working on this phase; another session asks it and does not edit this file. Rules and limits: `docs/plan.md`.

Done when: raw idea → spec → tasks without a terminal; 3 duplicates get a link; a fact given once is not asked again. Met on fakes and local Postgres. A real Lead passed the five intake cases. The full chat-to-tracker check is open.

## Status

- Last done: Q1a held an unreserved job across a service restart. Q2 and M5 passed on the stable Mac. The tracker approval boundary passed on Postgres with the tracker twin.
- Next: the [tracker acceptance check](../live-checks.md#tracker-projection) needs a real adapter and a test project. Q1b remains open for [funded runs](Q1a-monthly-credit-pools.md).
- Open: L3a scheduling passed with a fake Lead on Postgres and needs a real Lead run. M4 tenant isolation, S1 on Codex, voice, support and tracker checks remain open. The owner deferred Slack. Preview acceptance belongs to phase 6 P5.

## Items (owner's audit follow-up, 2026-11-25, and its follow-ups)

The owner's audit follow-up (2026-11-25), then the phase's own items.

- [x] A1–A2. Honest plan and status, `docs/decisions.md`; `make drill`: restore, restart, no token, no egress; green on the owner's Mac 2026-11-25 (22 of 22)
- [x] A3–A5. Design-vs-code check (`cmd/nightshift/design_test.go`); rule for unverified external facts (AGENTS.md, Lead role); limits measured by source, usage query in the runbook (decision 10)
- [x] Q0. Promise checks: `cmd/nightshift/promises_test.go` (prompt commands, role skills and keys, schema fields); skill `self-audit`; AGENTS.md "Done means checked"
- [ ] Q1. Wire or remove every schema field in the allowlist of `promises_test.go`, one PR per group (wired: `project.protected`, `tenant.language`, quiet hours #325, `remind_after_hours`, `sla_automerge_hours` and `capacity.previews` (phase 6); owner 2026-11-26: build the rest); the check matches key names, not paths, so a common word hides a gap; command scripts are found by a simple rule (a program with a directory, or a script extension), computed paths must be listed in `protected`
- [x] Q1a. Hold worker jobs with no guaranteed charge bound. A [service restart on Postgres](../../live-checks/Q1a-pool-admission.md) kept the capped job queued and leased the uncapped job.
- [ ] Q1b. [Reserve a guaranteed maximum charge before a funded run](Q1a-monthly-credit-pools.md) and reconcile actual spend across tenants and retries.
- [x] Q2. Browser acceptance: own skill `own/browser-acceptance` (no vendoring), Chromium and a pinned `playwright-core` baked into the worker image, Verifier switched (#329, owner 2026-11-26). The Mac built the worker image. On `factory-stable-2026-11-26.5`, Playwright 1.56.1 launched Chromium 156.0.8078.108 as uid 20001 and read the test page heading. See [worker build and launch evidence](../live-checks.md#passed).
- [x] H1. Digest "Factory health"
- [x] R1. A timeout per conductor step: `Ticker.StepTimeout` (30 minutes) cuts a hung step, logs it and leaves the task for the next tick; a canceled git gets SIGTERM before SIGKILL so its lock files go; T1. Open: a wait reason in the book, not only the log (ADR 0018)
- [x] R2. Every conductor and train side effect repeats safely (ADR 0018): `OpenPR` returns the open pull request of the branch (#263); `sideeffects_test.go` lists each call with the reason a repeat is harmless and fails on a new one; the hand-over calls repeat on the twin. Not verified on real GitHub
- [x] C1. Card labels follow the chat language and the spec card has its own kind `spec_for_approval` (live check 2026-11-26); a log line "spec coverage checked" shows that N5 ran. T4 on `fake`; Slack rendering not verified
- [ ] M4. Per-tenant HOME isolation (live check of #310 passed 2026-11-26): the separation is by path only, one uid, so a job can read another tenant's files. Needs a uid per tenant or 0700 directories with separate users; decide with an ADR
- [x] M5. `deploy/upgrade.sh` moves pre-#310 state (`.claude`, `.codex`, `.claude.json`) out of the shared `/home/agent` into `.pre-tenant-state/<time>` (root, 0700) with `deploy/legacy-home.sh` in a one-off root container; it never deletes; T6 `TestLegacyHomeStateIsMovedAndKept` on a temp dir. The Mac upgrade archived the existing entries and the stable worker denied uid 20001 access. See [M5 evidence](../live-checks.md#passed)
- [ ] S1. `feat(runner): one supervisor for every harness` (ADR 0019) — stall, repeat, token and dollar rules on `claude` and `codex` events, group stop, limits per role and tenant; T1. Live on the Mac 2026-11-26 (claude, task #44, limit 200): stopped mid-run at 3960 tokens. Usage arrives once per model message, so one long message overshoots; an estimate from text deltas is later. A stopped run records cost only from a provider `price`. Open until codex runs live (T8)
- [x] S2. `feat(summary): a task timeline` — `nightshift status --task <id>` merges `events` and every job of the task (who, when, exit, time, tokens, cost, why it stopped); the Grafana "Task timeline" dashboard reads `book.dash_task_timeline` as `grafana_ro`; T1, T3. Live 2026-11-26 on the stable instance (606ae8b): task 1004 matches the CLI; `grafana_ro` is refused on `book.jobs`. The dashboard shows no model or effort (L5 is CLI only)
- [x] S3. `feat(dispatch): model and effort per role` (ADR 0020) — role, tenant `roles`, provider pin; claude and codex flags; Lead and Verifier pinned at `high`; T1. Live 2026-11-26 on the Mac: task #42 ran the asked model and effort
- [x] S4. `feat(harness): codex on a ChatGPT plan` (owner 2026-11-26) — provider `codex-sub` (`auth: chatgpt`, one session, no `base_url`), `CODEX_ACCESS_TOKEN` without an API key and a forced ChatGPT login, egress `chatgpt.com` and `auth.openai.com`; T1. Live check pending (`live-checks.md`); its run also closes S1's codex part

- [ ] L3a. Carry the start time and priority from an approved Lead spec to every queued task. The CLI door and local Postgres exercise single and multi-task schedules. The [real Lead check](../../live-checks/L3a-lead-task-schedule.md) is open.

## Task intake (owner request, 2026-11-26; `docs/research/task-intake.md`)

Phase 5 Lead work; each starts after the phase 5 thread's open PRs on `roles/` and `lead.go` merge.

- [x] N1. `feat(lead): understood line` — `understood` (at most 25 words) in `schemas/lead-reply.json`, shown on question and spec cards; T1, T4.
- [x] N2. `docs(skills): task clarity check in challenge` — goal, done criterion and scope; an empty one means one question with options, not a spec.
- [x] N3. `feat(lead): quote the owner per criterion` — `source_quote` in `schemas/spec.json`; the Verifier gets the owner's message beside the spec.
- [x] N4. `test(evals): Lead intake cases` — frozen cases `several-asks`, `garbled-name` (ask) and `clear-task`, `over-specified` (spec); `expect: ask|spec` is graded by `eval run --role lead`. N7 (done): multi-turn cases (`thread`) grade a correction with `keep` and `drop`.
- [x] N5. `feat(lead): coverage check` — code check that each number, name and negation the owner wrote is in the spec; a miss goes back once (`docs/research/safety-nets.md`); T1.
- [x] N6. `feat(lead): independent reading` — role `reader` sees only the owner's messages and lists what "done" means; what the spec lacks goes on the spec card. Off without a `reader` chain; T1, T4 on the fake harness. Switch it on only if planted-misreading cases show a benefit on a real run (not measured).
- [x] N8. `feat(lead): problem reports` — skill `support`, spec field `report`, buttons Create task and Not needed, two eval cases. Open: a live reproduction.
- [x] L5. `feat(summary): Lead run record` — model and effort asked and the model run (`usage.model`) in the S2 timeline; pins are S3.

## Scope and open work

Built: `roles/lead.yaml` stub, reply check, the minimal loop (item 18). Built: skills `challenge`, `grilling`, `to-spec`, `to-tickets`, `hidden-scenarios` (own; vendor `grilling` later), Builder's `needs_research` rule in its prompt, which sends the task back to Lead (twice at most). Task dependencies built: an approved spec becomes tasks with `book.task_deps`, a task waits in `queued` until its dependencies are `released`, a cancelled one blocks it (the owner's go-on decision button is open). Project memory page storage built (design section 13, ADR 0005): `book.memory_pages` with `revision`, 40 entries and 4 KB cap, `nightshift memory show|remember`; Lead's `memory[]` is accepted only with a quote of an owner message or a repository file as source. Morning digest built (ADR 0013): `nightshift digest` lists merges and decisions with reasons and revert links; the 09:00 post to the owner is built (Slack adapter; `/ns status` still open). The prune also drops an entry whose text `AGENTS.md`, `CLAUDE.md`, `project.yaml` or its cited file now states (reason `in repo: <file>`). Role overrides (`.nightshift/roles/<name>.yaml`, instructions and skills only) built. `/ns status|memory|remember` in chat are built. Also built (2026-11-26, on `fake` and local Postgres only): the book search and duplicate answer (`nightshift search`, `/ns search`, `duplicate_of`), hidden scenarios storage (`lead_verifier` visibility), the doc-lint loop for the PRD and ADR drafts of a spec, and the tracker projection (`internal/control/tracking/tracker.go`). The tracker twin verified that tasks project after approval, including redelivery of a previously queued draft. A real tracker is still needed. Open: #support, voice, M3 part 2, Q1 (none is needed for the Done criterion). Done: raw idea → spec → tasks without a terminal; 3 duplicates get a link; a fact given once is not asked again.
