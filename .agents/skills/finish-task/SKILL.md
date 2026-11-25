---
name: finish-task
description: Wrap up a task on this repo: update plan and status, run make verify, open a draft PR with the required description, mark it ready and merge it under the merge rule. Use before every push and PR.
---

# Finish a task

Start of a session: read `docs/status.md`, then `docs/plan.md`, then take the next open item of the current phase. Work outside the plan needs the owner's OK and a plan update first.

## Before the PR

1. One task, one branch `feat/<topic>`, `fix/<topic>` or `docs/<topic>`; never push to `main`. Keep the diff under about 400 lines.
2. `make tools` first: a local linter that differs from the `mise.toml` pin hides errors that CI then reports.
3. Tick the item in `docs/plan.md` (max 150 lines). Rewrite `docs/status.md` (max 40 lines; last done, next, blockers, questions for the owner; no history). A 41st line fails `repo-check` on main.
4. A change to what a person runs, configures or keeps updates `README.md`, `docs/runbook.md` or `docs/secrets.md` in the same PR.
5. `make verify` green. Never skip or weaken a check. Re-run only what failed while fixing.

## Test levels

Logic: T1-T2. Flow change: a T4 scenario. Deploy, runner or egress: T6 escape tests. A fixed bug gets a regression test. Tests never call a real model.

## The PR

Conventional Commit title. Description: what and why (3-5 lines), design section, test levels, risks and rollback, open questions. Draft until `make verify` is green. Gated paths (`.github/`, `deploy/`, `schemas/`, `mise.toml`, `mise.lock`, `Makefile`, `AGENTS.md`, `CLAUDE.md`, dependencies, secrets, egress, gates, review policy) need the label `needs-owner-review` and are not self-merged.

## Merge

AGENTS.md lets a session merge its own PR in every phase once `make verify` and CI are green and no "Ask before you do it" item is open. Batch gated items for the owner in `docs/status.md`.
