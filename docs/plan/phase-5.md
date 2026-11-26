# Phase 5 — Lead: planning and support

Owned by the session working on this phase; another session asks it and does not edit this file. Rules and limits: `docs/plan.md`.

Done when: raw idea → spec → tasks without a terminal; 3 duplicates get a link; a fact given once is not asked again. Met on fakes and local Postgres; nothing ran with a live Lead.

## Status

- Last done (2026-11-26): standards in the worker image, memory prune, tracker projection (twin only), `nightshift search`, duplicate links, hidden scenarios, PRD and ADR drafts through doc-lint (#276–#287); Lead intake and safety nets N1–N8 (#288–#317), including an optional independent reader (off without a `reader` chain). Research: R1 (a timeout per conductor step, #265) and R2 (repeat-safe side effects, #263, #264); ADR 0018 (no Temporal), S1 (ADR 0019), S2, S3 (ADR 0020) merged. All on the `fake` harness, none with a live Lead, Slack, tracker or GitHub.
- Next: L5 (Lead run record), then the owner's test of the factory.
- Open: voice, a live #support reproduction, Q1 (rest), Q2. S1 (Mac re-check after #305), S2 and S3 wait for Mac live checks.
- Live check wanted before the owner's own project: one idea in Slack through a real Lead to a spec card, a duplicate, and the tracker.

## Items (owner's audit follow-up, 2026-11-25, and its follow-ups)

The owner's audit follow-up (2026-11-25), then the phase's own items.

- [x] A1. Honest plan and status, `docs/decisions.md`
- [x] A2. `make drill`: restore, restart, no token, no egress; green on the owner's Mac 2026-11-25 (22 of 22)
- [x] A3–A5. Design-vs-code check (`cmd/nightshift/design_test.go`); rule for unverified external facts (AGENTS.md, Lead role); limits measured by source, usage query in the runbook (decision 10)
- [x] Q0. Promise checks: `cmd/nightshift/promises_test.go` (prompt commands, role skills and keys, schema fields); skill `self-audit`; AGENTS.md "Done means checked"
- [ ] Q1. Wire or remove every schema field in the allowlist of `promises_test.go`, one PR per group (wired: `project.protected`, `tenant.language`); the check matches key names, not paths, so a common word hides a gap; command scripts are found by a simple rule (a program with a directory, or a script extension), computed paths must be listed in `protected`
- [ ] Q2. `playwright/playwright-cli` is vendored or dropped from the Verifier (prompts no longer name `ns search`: `nightshift search` is built and Lead gets hits in its prompt; role keys `inputs`, `fresh_context`, `on` are removed)
- [x] R1. A timeout per conductor step: `Ticker.StepTimeout` (30 minutes) cuts a hung step, logs it and leaves the task for the next tick; a canceled git gets SIGTERM before SIGKILL so its lock files go; T1. Open: a wait reason in the book, not only the log (ADR 0018)
- [x] R2. Every conductor and train side effect repeats safely (ADR 0018): `OpenPR` returns the open pull request of the branch (#263); `sideeffects_test.go` lists each call with the reason a repeat is harmless and fails on a new one; the hand-over calls repeat on the twin. Not verified on real GitHub
- [ ] S1. `feat(runner): one supervisor for every harness` (ADR 0019) — stall, repeat, token and dollar rules on `claude` and `codex` events, group stop, limits per role and tenant; T1. Mac re-check 2026-11-26: tokens undercounted, fixed (#305). Open until the Mac checks both stream formats live (T8)
- [ ] S2. `feat(summary): a task timeline` — done: `nightshift status --task <id>` merges `events` and every job of the task (who, when, exit, time, tokens, cost, why it stopped); the Grafana "Task timeline" dashboard reads `book.dash_task_timeline` as `grafana_ro`; T1, T3. Open until the dashboard shows a task on the Mac's obs profile
- [ ] S3. `feat(dispatch): model and effort per role` (ADR 0020) — role, tenant `roles`, provider pin; claude and codex flags; Lead and Verifier pinned at `high`; T1. Open until a subscription run on the Mac

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

## Scope and open work

Built: `roles/lead.yaml` stub, reply check, the minimal loop (item 18). Built: skills `challenge`, `grilling`, `to-spec`, `to-tickets`, `hidden-scenarios` (own; vendor `grilling` later), Builder's `needs_research` rule in its prompt, which sends the task back to Lead (twice at most). Task dependencies built: an approved spec becomes tasks with `book.task_deps`, a task waits in `queued` until its dependencies are `released`, a cancelled one blocks it (the owner's go-on decision button is open). Project memory page storage built (design section 13, ADR 0005): `book.memory_pages` with `revision`, 40 entries and 4 KB cap, `nightshift memory show|remember`; Lead's `memory[]` is accepted only with a quote of an owner message or a repository file as source. Morning digest built (ADR 0013): `nightshift digest` lists merges and decisions with reasons and revert links; the 09:00 post to the owner is built (Slack adapter; `/ns status` still open). The prune also drops an entry whose text `AGENTS.md`, `CLAUDE.md`, `project.yaml` or its cited file now states (reason `in repo: <file>`). Role overrides (`.nightshift/roles/<name>.yaml`, instructions and skills only) built. `/ns status|memory|remember` in chat are built. Also built (2026-11-26, on `fake` and local Postgres only): the book search and duplicate answer (`nightshift search`, `/ns search`, `duplicate_of`), hidden scenarios storage (`lead_verifier` visibility), the doc-lint loop for the PRD and ADR drafts of a spec, and the tracker projection (`internal/dispatch/tracker.go`, twin only). Open: #support, voice, M3 part 2, Q1, Q2 (none is needed for the Done criterion). Done: raw idea → spec → tasks without a terminal; 3 duplicates get a link; a fact given once is not asked again.
