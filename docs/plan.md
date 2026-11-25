# Plan

Phases and their "Done" criteria are defined in `docs/design.md` (section 22).
This file tracks the current phase only. Limit: 150 lines.
Rules: one item = one PR, under ~400 changed lines. Tick an item in the PR that completes it.
Add an item only if it follows from the design and the owner agreed.

Phase 0 (skeleton and contracts) is done: PRs #1–#13.

## Current phase: 2 — runner, harnesses and isolation

Phase 1 is done in code (PRs #14–#29). Its Slack check (item 9) moved to the owner's local test:
`docker compose up`, the bot answers in a thread, a restart keeps state, a restore works.

- [x] 1. `feat(book): add jobs, job tokens, harness sessions and artifacts` — lease, attempts,
  token hashes only; T3
- [x] 2. `feat(jobsapi): serve claim, heartbeat, events and complete` — worker token checked
  against its sha256, job tokens scoped to task and role and expiring with the lease; T1, T3
- [x] 3. `feat(gitops): add repository mirrors and task worktrees` — bare mirror in `repos`,
  one worktree per task, patch applied and pushed only from `nightshift`; T1 on local git
- [x] 4. `feat(runner): add nightshift work` — claim loop, job directory, harness run as the agent
  user, schema check, patch and artifact sha256, complete; e2e on `fake`: job → worktree → result
  → artifacts
- [x] 5. `feat(harness): add the claude adapter` — `claude -p` with JSON schema, explicit
  permissions and settings, `--bare` for API providers, subscription env scrubbed; T1 on a stub
  binary, T8 opt-in
- [x] 6. `feat(dispatch): add provider router and task budget` — providers `claude-sub`
  (`concurrency: 1`), `claude-api`, `claude-cheap`, switch on limit, park on exhaustion,
  harness sessions per (task, role); T1, T3
- [x] 7. `feat(deploy): add the worker image and isolated networks` — users runner and agent,
  agent HOME volume, networks `core`, `jobs`, `ci`, `ui`, Smokescreen `egress` and `egress-ci`;
  T6: no GitHub token, no host outside the allowlist, no local network, no Postgres
- [x] 8. `test(jobsapi): a Builder token never sees hidden scenarios` — T6
- [x] 9. `feat(deploy): add the obs profile` — `lgtm`, Claude Code telemetry, `TRACEPARENT` from
  the runner so agent spans land in the task trace
- [x] 9a. `feat(harness): add the codex adapter` — OpenAI and any Responses API endpoint (DeepSeek) by
  `base_url`; vendor-neutral prompts; keys in `deploy/secrets/harness/`
- [x] 10. Done check: a real `claude` job in Grafana. The task trace holds the job, turn and check
  spans and the agent logs; Claude Code starts its own traces for its spans in `-p` mode, so
  model-call details are read from the logs (recommended reading, the owner can reopen it)

Owner requests outside the phase list: [x] repo hygiene: `SECURITY.md`, `CONTRIBUTING.md`, `.editorconfig`; [x] dev skills for agents working on this repo in `.agents/skills/` (`.claude/skills` links there; the factory's role skills stay in `skills/`); [x] Go style standard (`standards/industry/go/`) and stricter
`.golangci.yml`; [x] secrets hardening before item 9: `book_backup` role instead of the superuser,
no `os.Getenv` in Go code, URL passwords masked, rotation steps in the runbook. [x] Engineering disciplines map in design section 9 (docs only; work lands in phases 4 and 6).
[x] Design update from the industry comparison: lighter phase 3 (team path to phase 8, Home tab
to phase 7), two automatic rounds per layer, `remediation` in findings, readiness in `tenant doctor`.

## Phase 2 done when

- An e2e test on `fake` passes "job → worktree → result → artifacts".
- A real `claude` job shows in Grafana as one trace.
- The agent finds no GitHub token and cannot reach a host outside the allowlist, the local
  network or Postgres; a Builder token does not get hidden scenarios; T6 tests check all of this.

## Current phase: 3 — Builder, checks, gates

Phase 2 is done in code except item 10 above (the owner's key run). Items follow design section 22.

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
- [ ] 10. GitHub and Linear twins; T4 and T5 scenarios (a third red check escalates). Steps: [x] runner runs
  `verify` as the agent user and sends findings; [x] conductor, building leg (queued → build job → checks
  → patch → PR → CI); [x] conductor, review gate (policy decides human review), hand-off and train; [x] conductor: Verifier leg (read-only job on the pushed head, `dispatch.Decide`, findings back to Builder, a raised blast radius holds for a person; off without a verifier role and provider); [x] `dispatch.CheckLeadReply` (60 words, options, recommendation; Lead stage itself open); [ ] previews, acceptance and review reminders; [x] Builder jobs carry project instructions and the L0 checklist (`internal/standards`); [x] serve loop that ticks the conductor (`--repos`, off by default); [x] T4 integration scenarios on the GitHub twin (return after red check, third red escalates, request changes, rebase conflict, cancel, human review, hand-off); [ ] Linear twin; [ ] YAML scenario runner and T5 failures
- [x] 11. (discover, the skill, `tenant doctor` and `tenant init --open-pr` done; the PR carries `.nightshift/project.yaml`, AGENTS.md and standards sync come with phase 4) `tenant init --discover`, `onboard-tenant`, `tenant doctor`; discovery reports the stacks it
  finds, and `tenant doctor` fails when the worker lacks a stack's toolchain
- [x] 12. `feat(runner): assemble role context` — role instructions, `project.yaml: instructions` and
  the L0 checklist in one system file, selected skills in the job directory (design section 12);
  T1 golden on `example-solo`. Without it Builder runs on the task prompt alone
- [ ] 13. Done check on real infrastructure: `docker compose build` and `up`, one real Builder task
  on the `nightshift` tenant end to end, before Nightshift connects as a tenant of itself

- [x] 14. `feat(memory): findings and agent notes in the book` — `book.findings` and `book.notes`,
  `notes[]` in Builder and Verifier results (`schemas/note.json`), stored as proposed with
  provenance, never put into a prompt; `LEARNINGS.md` at 100 lines (owner-approved start of memory)
- [x] 15. `docs: package notes and a Lead stub` — `internal/*/AGENTS.md` (40 lines), `roles/lead.yaml`
- [ ] 16. `feat(runner): session handoff` — `handoff` in Builder results (`schemas/`, owner review),
  stored per (task, role), fed to a fresh session after a restart or provider switch, minimal one
  from `events` when missing; T1 and a T4 scenario where the runner dies mid-task (design section 12)

## Next phases

Phase 4 adds mutation testing of the core (go-mutesting or similar) as a nightly job, and the
`go`, `python` and `ts` stack packs (`stacks/<name>/pack.yaml`: toolchain, L1 tools; loaded by
the runner and `tenant doctor`, done) and their standards cards (go, python, ts: done); the polygon needs them. `check.DocLint` (structure of a document) is done as a library; wiring it into `nightshift check` and the document quality loop is open. The worker image is Go-only, so
python and ts tools are skipped until its toolchains are added.
Search: `internal/search` and `book.documents` (full text always; vectors and RRF only with `search.embedder` and the pgvector column) are done; indexer job, `ns search` and the Embedder provider are open.
`internal/learn` (recurring findings, worst first, at most three a night) is the deterministic half of the nightly lessons loop; the model half and the PRs it opens are open.
Phase 5 also builds project memory (design section 13): the capped page with a revision, `/ns remember`, `memory[]` in Lead's result, the nightly prune.
Phases 4–10 start only when the previous ones hurt without them (design section 22).
