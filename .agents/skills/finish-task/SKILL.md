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

Conventional Commit title. Description: what and why (3-5 lines), design section, test levels, risks and rollback, open questions, and a basis line (Checked / Inferred / Confidence, see AGENTS.md "Evidence"). Your final report to the owner ends with the same line; say what you ran and read, not what you remember. Draft until `make verify` is green. Gated paths (`.github/`, `deploy/`, `schemas/`, `mise.toml`, `mise.lock`, `Makefile`, `AGENTS.md`, `CLAUDE.md`, dependencies, secrets, egress, gates, review policy) need the label `needs-owner-review` and are not self-merged.

## Independent review

A PR that changes code is reviewed by an agent that did not write it before the merge (owner's rule of 2026-11-25, AGENTS.md). Run a fresh subagent with `/code-review` at high effort on the PR diff; do not review your own change in the session that wrote it. Fix every confirmed bug and add a regression test for it, then run the checks again; the findings that you reject get a one-line reason in the PR. Put `Reviewed by: (reviewer), N findings, M fixed` in the PR description. Docs-only PRs skip this step.

## Merge

AGENTS.md lets a session merge its own PR in every phase once `make verify` and CI are green, the independent review is done and no "Ask before you do it" item is open. Batch gated items for the owner in `docs/status.md`.

## Never stop to wait

A session does not end its turn to wait for CI. After the PR is open, poll its checks in the same turn (the check runs of the PR, every minute or so), fix a red check, merge on green and take the next item of `docs/plan.md` at once. Stop only when blocked on the owner (a new account, a paid service, new egress, an irreversible step) or on a usage limit; after a limit, resume when it resets. This is the owner's rule of 2026-11-25.
