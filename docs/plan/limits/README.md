# Stream — limits and the night queue

Owned by the session working on this stream; another session asks it and does not edit this file. Rules and limits: `docs/plan.md`.

Done when: a limit parks the job, a role chain waits or falls back, tasks are planned ahead (ADR 0012).

## Status

- Last done: L1–L5 are done (the Lead run record is item L5 of `docs/plan/phase-5/README.md`, a different item).
- Next: L6.

## Items

One item per PR, in order; L2 changes `schemas/` and needs the owner's review.

- [x] L1. `fix(dispatch): a limit parks the job, not the task` — limit and reset in the book, no escalation; T1, T3
- [x] L2. `feat(dispatch): wait or fall back by the role chain` — `max_wait`, `tier`, per-task `--allow-fallback`; schemas (owner review)
- [x] L3. `feat(cli): plan tasks ahead` — `not_before`, `priority` on `task add`, honored by the conductor; migration; T3.
  Left for Lead's `to-tickets` (phase 5, dependencies phase 10): the same fields on Lead tasks; `nightshift queue`
- [x] L4. `feat(summary): limits in the morning summary` — limit hits per provider and fallback jobs in `summary.Build`; the 09:00 job that sends it is not wired yet
- [x] L5. `docs(skills): own skill plan-night-queue` — unattended-ready tasks, reading the queue
- [ ] L6. `feat(dispatch): a context guard for long sessions` — a Builder or Verifier session resumed past a context size starts fresh from its handoff (design section 12); Lead's transcript is summarised past a size instead of growing without a cap; the size comes from the Mac live run, not a guess (`docs/retro/2026-11-26-long-threads.md`); T1, T4
