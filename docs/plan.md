# Plan

Phases and their "Done" criteria are defined in `docs/design.md` (section 22).
This file tracks the current phase in detail and lists every later phase with its exit criterion. Limit: 150 lines.
Rules: one item = one PR, under ~400 changed lines. Tick an item in the PR that completes it.
Add an item only if it follows from the design and the owner agreed.

Phases 0–2 are done (PRs #1–#110). Phase 0: skeleton and contracts. Phase 1: book, core, Slack; its
restart and restore checks run on the owner's machine. Phase 2: runner, harnesses (`claude`, `codex`,
`fake`), providers and budget, isolation (T6), the `obs` profile. Phase 2 item 10 is ticked on the
recommended reading: Claude Code's own spans sit in separate traces, so model-call details come from
the logs (the owner can reopen it). Owner requests outside the phase lists are also done: repo hygiene
files, dev skills in `.agents/skills/`, the Go style standard, secrets hardening, the engineering
disciplines map and the lighter phase 3.

## Current phase: 3 — Builder, checks, gates

Items follow design section 22. By the owner's decision of 2026-11-25 phase 3 also builds a minimal
Lead (spec and approval in the Slack thread) and the Verifier's review leg on the `fake` harness; live
runs of both wait for the owner. Preview, acceptance and the full Lead stay in phases 6 and 5.

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

## Setup automation (owner request)

A first start of an instance is a checklist the factory checks and mostly does itself; only
what needs the owner's login or a secret stays manual (`docs/setup.md`). One item, one PR,
in this order.

- [x] S1. `feat(doctor): add nightshift doctor` — one line per setup step with ok or the fix:
  config, secrets, book, code host sign-in and installation, CI on each repository, chat
  tokens; `--worker` for the agent CLIs, credentials and egress; T1
- [x] S2. `feat(cli): add nightshift task add` — a queued task without SQL, with its event; T3
- [x] S3. `feat(setup): create the GitHub App from a manifest` — one confirmation on GitHub, then
  the id and key land in the secrets; T1 on the twin
- [x] S4. `feat(setup): create the chat app from its manifest` — a pre-filled link; tokens stay manual
- [x] S5. `feat(setup): add nightshift setup` — asks for ids, writes config, providers and `.env`,
  then points to the doctor, which needs the running stack; T1

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

## Phase 3 done when

All T4 and T5 scenarios are green; the CI check finds no project or service names in the core; a Slack
message (through Lead, once item 18 lands) → draft PR → L1–L3 green → a card mentioning the owner; a red
check returns the task to Builder; no merge without the owner's Approve; a Builder patch to `.github/` is
rejected; two parallel PRs merge in turn; `nightshift` killed mid-merge finishes after a restart; a task
survives a runner restart through the handoff. Order inside the phase: 18, 16, 17, the Linear twin and
T5 runner (10), then 13.

## Later phases (design section 22 has the full lists)

Each starts only when the previous Done criterion is met. Pieces already built ahead of their phase
are noted; they are not repeated in the phase's open work.

- **4. Standards and the document quality loop.** Built: `go`, `python`, `ts` stack packs; their
  standards cards; doc-lint in `nightshift check` (ADR and PRD rules, `check.DefaultDocKinds`); discovery reads stacks from the packs' `detect` and names languages without a pack; a job with nothing to check says so (`check.nothing_to_run`). Open: standards library and `standards sync`, Vale and lychee, L4 rubrics, templates (PRD, ADR, design), the polygon
  and `nightshift eval`, `skills sync`, `nightshift deps check`, the L2 research-record finding and own skills `research-options`, `build-vs-buy` (ADR 0007), python and ts toolchains in the worker image. Owner (2026-11-25): also stack packs `java-maven`, `java-gradle` and `js`, and skills `web-dev` and `mobile-dev` (mobile means React Native, JS-level checks on Linux CI; native Android and iOS builds and skills, simulators and device e2e are "Later"). (Mutation tests are in design "Later"
  while an older plan line put them here: the owner picks.) Done: a planted weak PRD gets L1 and L4 findings; planted code defects are caught by L1 or L4;
  the polygon runs by `evals/selection.yaml`.
- **5. Lead: planning and support.** Built: `roles/lead.yaml` stub, reply check, the minimal loop (item
  18). Open: skills `challenge`, `grilling`, `to-spec`, `to-tickets`, hidden scenarios, dependencies, `build-vs-buy` in `challenge` and Builder's `needs_research` stop, the
  project memory page and `/ns remember` (design section 13, ADR 0005), docs PRs, tracker projection,
  the indexer with `ns search` (the retrieval library exists), #support, voice. Done: raw idea → spec →
  tasks without a terminal; 3 duplicates get a link; a fact given once is not asked again.
- **6. Verifier, preview, production.** Built: Verifier role, verdict and the review leg on `fake`.
  Open: live review and acceptance through playwright-cli, preview hooks, #deployments card, `release`,
  `smoke`, `rollback`, `deploy/upgrade.sh`. Done: a Slack task ends with an evidence card; Merge deploys;
  a failed smoke rolls back on its own.
- **7. Self-improvement.** Built: `internal/learn` clustering. Open: the model half of nightly learn and
  its PRs, E2/E3 evals, taxonomy, autonomy ladder, Home tab. Done: a planted recurring defect becomes a
  tool rule overnight; a skill that degrades the polygon is rejected.
- **8. The `team` profile.** Starts with the first team tenant: `handed_off` path, second instance,
  Jira and Confluence, `copilot`. Done: `example-team` scenarios green; two instances isolated (T6).
- **9. Hybrid search on.** Built: nullable embedding column, RRF code. Open: an embedder, backfill.
  Done: hybrid beats full text on the 20-question search eval.
- **10. New projects, multi-repository tasks, dark mode.** Done: a new project connects in under an
  hour; a two-repository spec yields linked PRs; a week of `low` and `medium` tasks without clicks.
