# Plan

Phases and their "Done" criteria: `docs/design.md`, section 22. This file lists the open work. Limit: 150 lines.
Rules: one item = one PR, under ~400 changed lines; tick it in that PR. Add an item only if it follows from the design and the owner agreed.

Phases 0–4 are closed in code (PRs #1–#190). The live Done
checks of phases 1 (restore, restart) and 2 (no token, no way past the allowlist) passed on a real stack
with `make drill` on 2026-11-25 (`docs/status.md`). Phase 2 item 10 is ticked on
the recommended reading (model-call details come from the logs). Open owner decisions: `docs/decisions.md`.

## Current phase: 5 — Lead: planning and support

Phase 5's work is under "Later phases"; below is the owner's audit follow-up (2026-11-25).

- [x] A1. Honest plan and status, `docs/decisions.md`
- [x] A2. `make drill`: restore, restart, no token, no egress; green on the owner's Mac 2026-11-25 (22 of 22)
- [x] A3. Design-vs-code check: `cmd/nightshift/design_test.go` (compose example, subcommands)
- [x] A4. Rule for unverified external facts (AGENTS.md, Lead role); Playwright checked against its docs
- [x] A5. Limits: measured by source (plan limits are shared), usage query in the runbook, decision 10
- [x] Q0. Promise checks: `cmd/nightshift/promises_test.go` (prompt commands, role skills and keys, schema fields); skill `self-audit`; AGENTS.md "Done means checked"
- [ ] Q1. Wire or remove every schema field in the allowlist of `promises_test.go` (`project.protected` first: a project's own protected paths); one PR per field group
- [ ] Q2. Role keys `inputs`, `fresh_context`, `on`: the runner or conductor reads them, or the schema drops them

## Phase 3 — Builder, checks, gates (built; open: item 10 review reminders, D2, L4)

Items follow design section 22. Phase 3 also builds a minimal Lead and the Verifier's review leg on `fake` (owner, 2026-11-25);
live runs of both wait for the owner.

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
- [x] 11. (discover, the skill, `tenant doctor` and `tenant init --open-pr` done; the PR carries `.nightshift/project.yaml`, AGENTS.md and standards sync come with phase 4) `tenant init --discover`, `onboard-tenant`, `tenant doctor`; discovery reports the stacks it
  finds, and `tenant doctor` fails when the worker lacks a stack's toolchain
- [x] 12. `feat(runner): assemble role context` — role instructions, `project.yaml: instructions` and
  the L0 checklist in one system file, selected skills in the job directory (design section 12);
  T1 golden on `example-solo`. Without it Builder runs on the task prompt alone
- [x] 13. Done check on real infrastructure: `docker compose build` and `up`, one real task end to
  end. On the `nightshift` tenant a chat idea went through Lead, the owner's approval, Builder,
  CI, review and the merge train to `released` (#148)

- [x] 14. `feat(memory): findings and agent notes in the book` — `book.findings` and `book.notes`,
  `notes[]` in Builder and Verifier results (`schemas/note.json`), stored as proposed with
  provenance, never put into a prompt; `LEARNINGS.md` at 100 lines (owner-approved start of memory)
- [x] 15. `docs: package notes and a Lead stub` — `internal/*/AGENTS.md` (40 lines), `roles/lead.yaml`
- [x] 16. `feat(runner): session handoff` — `handoff` in Builder results (`schemas/`, owner review),
  stored per (task, role), fed to a fresh session after a restart or provider switch, minimal one
  from `events` when missing; T1 and a T4 scenario where the runner dies mid-task (design section 12)
- [x] 18. `feat(dispatch): Lead stage in Slack` — a raw idea in a thread becomes a short spec sent to
  the owner (`CheckLeadReply`: 60 words, options, a recommendation); Approve queues the task, Reject
  or an edit loops; the thread state is kept in the book; T4 scenario on `fake` with the Slack twin.
  Lead's results carry `handoff` (item 16); skills, hidden scenarios and dependencies are phase 5
- [x] 17. `fix(harness): report codex cost` — `cost_usd` is 0 today, so a pool budget cannot see
  its spend; read usage from the CLI output or price tokens from `providers.yaml`; T1

## Setup automation (owner request; done)

- [x] S1–S5. `nightshift doctor` (one line per setup step, `--worker`), `task add`, the GitHub App from a manifest, the chat app from its manifest, `nightshift setup`; T1, T3. Steps are in `docs/setup.md`.

## Dogfooding (owner request, 2026-11-25)

- [x] D1. `feat(scripts): run a stable factory instance` — `scripts/stable.sh` runs a tag as its own
  compose project; there `low` risk merges on green CI, the rest waits for the owner
- [ ] D2. Small tasks from this plan go through the stable instance, one at a time, each with an
  independent review; factory defects found are fixed in normal PRs

## Other doors besides Slack (owner request, 2026-11-25)

Slack stays the default. A tenant without Slack, or one who works from their own Claude, drives the
same task thread through the book (design section 15, ADR 0006). They start after phase 3 items 18, 16
and 17, one item per PR, in this order. The tenant schema value `inbox` needs the owner's review.

- [x] I1. `feat(chat): inbox adapter` — `Chat` on the book: cards and replies in the task thread,
  decisions recorded with their `source`; T1 and a T4 scenario (Lead spec, reply, approve) on `fake`
- [x] I2. `feat(cli): status, inbox, reply, approve` — beside `task add`; T3
- [x] I3. `feat(mcp): nightshift mcp over stdio` — tools `task_add`, `status`, `inbox`, `reply`,
  `approve` on the official Go SDK (dependency approved 2026-11-25); T1 on the tools
- [x] I4. `feat(mcp): remote door over HTTPS` — off by default, `127.0.0.1`, token with a short scope
  (no merge, rollback or settings); first check the connector sign-in (design section 25); T6
- [x] I5. `docs(setup): choose the door` — `nightshift setup` asks Slack or no Slack and prints the one
  command that connects a Claude client; `docs/setup.md`, `docs/secrets.md` and the
  `set-up-instance` skill follow it

## Limits and the night queue (owner request, 2026-11-25; ADR 0012)

One item per PR, in order; L2 changes `schemas/` and needs the owner's review.

- [x] L1. `fix(dispatch): a limit parks the job, not the task` — limit and reset in the book, no escalation; T1, T3
- [x] L2. `feat(dispatch): wait or fall back by the role chain` — `max_wait`, `tier`, per-task `--allow-fallback`; schemas (owner review)
- [x] L3. `feat(cli): plan tasks ahead` — `not_before`, `priority` on `task add`, honored by the conductor; migration; T3.
  Left for Lead's `to-tickets` (phase 5, dependencies phase 10): the same fields on Lead tasks; `nightshift queue`
- [x] L4. `feat(summary): limits in the morning summary` — limit hits per provider and fallback jobs in `summary.Build`; the 09:00 job that sends it is not wired yet
- [x] L5. `docs(skills): own skill plan-night-queue` — unattended-ready tasks, reading the queue

## Several tenants and agent-made configuration (owner request, 2026-11-25; ADR 0015)

One instance builds several projects at once; an agent configures a tenant through files and a PR. One item per PR, in order. The phase 5 thread owns `roles/` and `lead.go`; none of these edit them.

- [ ] C1. `feat(cli): nightshift config validate` — instance, providers, tenants and (`--repo-dir`) `.nightshift/` files against `schemas/`, plus references (providers, skills, packs, repos); no secrets or network; T1, T3
- [ ] C2. `docs(skills): own skill configure-tenant` — stack, skills, Lead instructions, gates, review mode via files and a PR, with C1 and `tenant doctor`; never secrets, `deploy/` or egress; runbook updated
- [ ] M1. `feat(dispatch): capacity.tasks per tenant, one loop per tenant` — a task over the cap stays `queued`; a slow tenant does not delay the others; T2, T3
- [ ] M2. `feat(book): claim fairly across tenants` — oldest job of the tenant with the fewest running jobs per `budget.weight`; T3 and a rapid property
- [ ] M3. `feat(runner): isolate worker state between tenants` — own mirror only, HOME and work cleaned between tenants; T6 (`deploy/`, runner)
- [ ] M4. `feat(config): code-host credential per tenant` — `deploy/secrets/tenants/<name>/github_token`, instance fallback; `docs/secrets.md`; T1
- [ ] M5. `docs(runbook): several projects on one laptop` — add a tenant (restart); raise `claude-sub` `concurrency` in `providers.yaml` within `weekly_share`, and a check that the sum of tenants' demand fits it. Later: reload without restart, a per-tenant provider share, MCP write tools through PRs only

## Phase 3 done when

See design section 22; the scenarios and gates are green in CI, item 13 ran on real infrastructure (#148).

## Later phases (design section 22 has the full lists)

Each starts only when the previous Done criterion is met. Pieces already built ahead of their phase
are noted; they are not repeated in the phase's open work.

- **4. Standards and the document quality loop.** Built: `go`, `python`, `ts` stack packs; their
  standards cards; doc-lint in `nightshift check` (ADR and PRD rules, `check.DefaultDocKinds`); discovery reads stacks from the packs' `detect` and names languages without a pack; a job with nothing to check says so (`check.nothing_to_run`). `standards list` and `standards sync` (base Go and Python configs from `standards/<set>/<stack>/files/`, never over a project's own file; the PR step comes with `tenant init`). Also built (ADR 0009, 0010): `mise install` and `project.yaml: setup` before the agent, run by the `toolchain` service with its own egress; Debian worker image; `tenant doctor` for a pinned toolchain; packs `js`, `java-maven`, `java-gradle`; own skills `research-options`, `build-vs-buy`, `web-dev`, `mobile-dev` (selected by `applies_to.deps`); `nightshift deps check`; `nightshift skills sync` (vendors `skills/vendor.yaml` by SHA, scans, writes a diff report; the starter list stays empty until the owner picks); L4 rubric cards (`standards/industry/docs/rubrics/`, `internal/rubric`, `check.RubricFindings` on a `Judge`; the live judge and the Verifier wiring are open). Open: project packs in `.nightshift/stacks/`, standards library, Vale and lychee, templates (PRD, ADR, design), the polygon
  and `nightshift eval`, the L2 research-record finding. Owner (2026-11-25): also stack packs `java-maven`, `java-gradle` and `js` (`rust` with the first Rust tenant), and skills `web-dev` and `mobile-dev` selected by dependencies (mobile means React Native, JS-level checks on Linux CI; native Android and iOS builds and skills, simulators and device e2e are "Later"). (Mutation tests are in design "Later"
  while an older plan line put them here: the owner picks.) Done: a synthetic Rust project passes `tenant doctor` and `verify` without a pack; a planted weak PRD gets L1 and L4 findings; planted code defects are caught by L1 or L4;
  the polygon runs by `evals/selection.yaml`. Polygon (owner 2026-11-25, ADR 0011): runs locally, a throwaway git repository per task, no GitHub; `eval run` built, 1 synthetic task. Closed 2026-11-25 (Done met); `skills sync` and the harness judge (`check --standards --judge-auth`) are in. Leftovers: the runner's rubric call needs `COPY standards` in the worker image (owner), Vale/lychee (`mise.lock` PR), the standards book index, the 10-task polygon set.
- **5. Lead: planning and support.** Built: `roles/lead.yaml` stub, reply check, the minimal loop (item
  18). Built: skills `challenge`, `grilling`, `to-spec`, `to-tickets`, `hidden-scenarios` (own; vendor `grilling` later), Builder's `needs_research` rule in its prompt. Task dependencies built: an approved spec becomes tasks with `book.task_deps`, a task waits in `queued` until its dependencies are `released`, a cancelled one blocks it (the owner's go-on decision button is open). Project memory page storage built (design section 13, ADR 0005): `book.memory_pages` with `revision`, 40 entries and 4 KB cap, `nightshift memory show|remember`. Morning digest built (ADR 0013): `nightshift digest` lists merges and decisions with reasons and revert links; the 09:00 post to the owner is built (Slack adapter; `/ns status` still open). Open: hidden scenarios storage, routing `needs_research` back to Lead, the
  memory page's `/ns` chat commands, `memory[]` in Lead's result (schema) and the prune rule for facts that moved into the repo, docs PRs, tracker projection,
  the indexer with `ns search` (the retrieval library exists), #support, voice. Done: raw idea → spec →
  tasks without a terminal; 3 duplicates get a link; a fact given once is not asked again.
- **6. Verifier, preview, production.** Built: Verifier role, verdict and the review leg on `fake`. Open, in order (design section 17 "Deploy targets", ADR 0014): P1 `Deployer` interface, `deploy/fake` twin, `capabilities` in `schemas/project.json` (owner review); P2 conductor steps `up`, `down`, `release`, `smoke`, `rollback` through a journal, T4 scenario with the automatic rollback; P3 adapters `script` and `external`; P4 `deployer` job and recipe lint, `compose-local` (decision 13 is taken), T6 escapes; P5 live acceptance through playwright-cli on the preview, hidden scenarios, #deployments card; P6 revert PR and frozen train on rollback, preview limits and teardown, `deploy/upgrade.sh`. `compose-ssh`, `kubernetes` and `load` wait for their first tenant.
  Done: on `compose-local` a Slack task ends with an evidence card and a torn-down preview; Merge deploys; a failed smoke rolls back on its own. Latent: after `revert_merged` the old merged `pr_number` would end the task at once (outside-merge watch); clear it at the `rolled_back` to `building` move.
- **7. Self-improvement.** Built: `internal/learn` clustering. Open: the model half of nightly learn and
  its PRs, E2/E3 evals, taxonomy, autonomy ladder, Home tab. Done: a planted recurring defect becomes a
  tool rule overnight; a skill that degrades the polygon is rejected.
- **8. The `team` profile.** Starts with the first team tenant: `handed_off` path, second instance,
  Jira and Confluence, `copilot`. Done: `example-team` scenarios green; two instances isolated (T6).
- **9. Hybrid search on.** Built: nullable embedding column, RRF code. Open: an embedder, backfill.
  Done: hybrid beats full text on the 20-question search eval.
- **10. New projects, multi-repository tasks, dark mode.** Done: a new project connects in under an
  hour; a two-repository spec yields linked PRs; a week of `low` and `medium` tasks without clicks.
