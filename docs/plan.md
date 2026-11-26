# Plan

The owner's one-page plan: every phase, its state and the folder that holds its work. Phase "Done" criteria: `docs/design.md`, section 22. Limit: 60 lines and 800 words.

## Current phase: 5 — Lead: planning and support

Phases 0–4 are closed in code (PRs #1–#190). The live Done checks of phases 1 (restore, restart) and 2 (no token, no way past the allowlist) passed on a real stack with `make drill` on 2026-11-25 (`docs/status.md`). Phase 2 item 10 is ticked on the recommended reading (model-call details come from the logs). Open owner decisions: `docs/decisions.md`. Phase 6 runs in a parallel thread beside phase 5's long tail (decision 16).

## Phases

| Phase | State | Work, status and Done criterion |
|---|---|---|
| 0–2 Skeleton, book, runner | closed | none open; live drill passed 2026-11-25 |
| 3 Builder, checks, gates | built | [`plan/phase-3/README.md`](plan/phase-3/README.md) |
| 4 Standards and the document quality loop | closed | [`plan/phase-4/README.md`](plan/phase-4/README.md) |
| 5 Lead: planning and support | current | [`plan/phase-5/README.md`](plan/phase-5/README.md) |
| 6 Verifier, preview, production | in progress, parallel | [`plan/phase-6/README.md`](plan/phase-6/README.md) |
| 7 Self-improvement | later | [`plan/phase-7/README.md`](plan/phase-7/README.md) |
| 8 The `team` profile | later | [`plan/phase-8/README.md`](plan/phase-8/README.md) |
| 9 Hybrid search on | later | [`plan/phase-9/README.md`](plan/phase-9/README.md) |
| 10 New projects, multi-repository tasks, dark mode | later | [`plan/phase-10/README.md`](plan/phase-10/README.md) |

Owner requests that cut across phases have a folder of their own:

| Stream | State | Work and status |
|---|---|---|
| Several tenants and agent-made configuration (ADR 0015) | open | [`plan/tenants/README.md`](plan/tenants/README.md) |
| Other doors besides Slack (ADR 0006) | open | [`plan/doors/README.md`](plan/doors/README.md) |
| Limits and the night queue (ADR 0012) | open (L6) | [`plan/limits/README.md`](plan/limits/README.md) |
| Live checks owed by merged work | open | [`plan/live-checks.md`](plan/live-checks.md) |

## How the plan is kept

- One phase or stream, one folder under `docs/plan/`. Its `README.md` holds the "Done" criterion, the "Status" block (last done, next, open) and the items. The session working on the phase owns that file: it ticks items, adds items that follow from the design, and rewrites "Status". Parallel phases edit different files, so they do not conflict.
- A session that does not own a phase proposes an item as a new file in that folder, `<id>-<topic>.md`: a title, a line `State: proposed`, the source, what to do, how to check. A new file never conflicts. After the owner agrees, the phase's session adds the item line to its `README.md` and sets `State: open`, then `State: done`.
- This file changes only when a phase starts or closes, and when a folder is added. It holds no item and no count.
- One item = one PR, under ~400 changed lines; tick it in that PR. Add an item only if it follows from the design and the owner agreed. A session that needs an item in a phase it does not own asks that phase's thread.
- Limits are per file: this file 60 lines and 800 words, each `README.md` and `live-checks.md` 200 lines and 4000 words, an item file 80 lines and 800 words (`scripts/repo-check.sh`). A file that outgrows it is split by topic, never compressed.
- Item ids are unique inside a folder. Start a folder from an existing one.
- `docs/status.md` holds only what holds across phases. A phase's own last done and next step go in its "Status".

## How threads are run

- One thread works one phase or stream folder and ends when its batch is merged. A new phase or stream gets a new thread.
- Hand off when any trigger fires: about 4 hours, about 10 merged PRs, context near half the window (about 250k tokens), a usage-limit stop, or a phase boundary. The numbers are chosen, not measured (`docs/retro/2026-11-26-long-threads.md`).
- A handoff is the rewritten "Status" block of your file and a brief of at most 10 lines to the coordinator: goal, next item, files owned, open PRs, what not to touch. Merge or close your PRs first. The coordinator starts the next thread or schedules a wake in the same turn.
- Phases run in order. A later phase starts early only when the earlier one has live checks left and nothing else; it is named here. Today: phase 6 beside 5 (decision 16).
- At most two threads change the repository at once, on files their briefs name as disjoint. Research, review and live-check threads never push to another thread's branch.
