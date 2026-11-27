# Phase 3 — Builder, checks, gates

Owned by the session working on this phase; another session asks it and does not edit this file. Rules and limits: `docs/plan.md`.

Done when: see design section 22: the scenarios and gates are green in CI, and item 13 ran on real infrastructure (#148). Built; open: item 10 review reminders and D2.

## Status

- Last done: items 1–18 except review reminders; one real task ran from a chat idea to `released` on the `nightshift` tenant (#148). Z1–Z5 (setup automation) and D1 (the stable instance) are done.
- Next: item 10 review reminders; D2 (small tasks through the stable instance); D3 (Stop hook self-audit, after a Mac live check).
- Open: nothing blocks it.

## Items

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
  `internal/observability/summary` builds it from the book; the Slack command and the 09:00 job arrive with the wiring
- [x] 10. GitHub and Linear twins; T4 and T5 scenarios (a third red check escalates). Steps: [x] runner runs
  `verify` as the agent user and sends findings; [x] conductor, building leg (queued → build job → checks
  → patch → PR → CI); [x] conductor, review gate (policy decides human review), hand-off and train; [x] conductor: Verifier leg (read-only job on the pushed head, `dispatch.Decide`, findings back to Builder, a raised blast radius holds for a person; off without a verifier role and provider); [x] Lead stage (chat idea → Lead job → reply or spec card → Approve/Edit/Reject buttons → Builder gets the spec; `lead.go`); [ ] review reminders; [x] Builder jobs carry project instructions and the L0 checklist (`internal/assurance/standards`); [x] serve loop that ticks the conductor (`--repos`, off by default); [x] T4 integration scenarios on the GitHub twin (return after red check, third red escalates, request changes, rebase conflict, cancel, human review, hand-off); [x] tracker twin (`internal/adapters/tracker/fake`; a real Linear adapter comes with the first tenant that uses it, phase 8); [x] YAML scenario runner (`TestScenarioHappyPath` replays `happy-path.yaml` and compares the events with a golden file); [x] more scenario files (`spec-rejected`, `changes-requested`; the runner replays every `*.yaml`)
- [x] 11. `tenant init --discover` (reports stacks), `onboard-tenant`, `tenant doctor` (fails without a stack's toolchain), `tenant init --open-pr`
- [x] 12. `feat(runner): assemble role context` — role instructions, `project.yaml: instructions` and
  the L0 checklist in one system file, selected skills in the job directory (design section 12);
  T1 golden on `example-solo`. Without it Builder runs on the task prompt alone
- [x] 13. Done check on real infrastructure: `docker compose build` and `up`, one real task end to
  end. On the `nightshift` tenant a chat idea went through Lead, the owner's approval, Builder,
  CI, review and the merge train to `released` (#148)

- [x] 14–18. Findings and notes in the book; package notes; session handoff; the Lead stage in Slack (`leadreply.Check`); codex cost. T1, T4 on `fake`.

## Conductor tests from `docs/prior-art.md` (Antithesis; decision 14)

- [ ] X1. `test(flow)`: liveness property, a rapid model of the conductor with outside events (merge or close on GitHub, cancel, CI result, worker death); every task not terminal has a next step or a recorded wait reason; T2
- [ ] X2. `test(scenarios)`: seeded faults in the twins (push, PR or merge fails at step N, a worker dies, a limit hits), the seed is printed and replayable, `sometimes` checks that a scenario reaches its state; T5; after X1

## Setup automation (owner request; done)

- [x] Z1–Z5. `nightshift doctor` (one line per setup step, `--worker`), `task add`, the GitHub App from a manifest, the chat app from its manifest, `nightshift setup`; T1, T3. Steps are in `docs/setup.md`.

## Dogfooding (owner request, 2026-11-25)

- [x] D1. `feat(scripts): run a stable factory instance` — `scripts/stable.sh` runs a tag as its own
  compose project; there `low` risk merges on green CI, the rest waits for the owner
- [ ] D2. Small plan tasks go through the stable instance one at a time, each reviewed; its defects get normal PRs
- [ ] D3. Stop hook self-audit (#253 closed; branch `claude/project-thread-itaacy`): reopen after Mac live check
