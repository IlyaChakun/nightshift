---
name: finish-task
description: Wrap up a task on this repo: update plan and status, run make verify, open a draft PR with the required description, mark it ready and merge it under the merge rule. Use before every push and PR.
---

# Finish a task

Start of a session: read `docs/status.md`, then `docs/plan.md`, then take the next open item of the current phase. Work outside the plan needs the owner's OK and a plan update first.

## Before the PR

1. One task, one branch `feat/<topic>`, `fix/<topic>` or `docs/<topic>`; never push to `main`. Keep the diff under about 400 lines.
2. `make tools` first: a local linter that differs from the `mise.toml` pin hides errors that CI then reports.
3. Tick the item in `docs/plan.md` (max 150 lines). Rewrite `docs/status.md` (max 40 lines and 600 words, skill `write-docs`; last done, next, blockers, questions for the owner; no history). A 41st line fails `repo-check` on main.
4. A change to what a person runs, configures or keeps updates `README.md`, `docs/runbook.md` or `docs/secrets.md` in the same PR.
5. Run the `self-audit` skill and keep its block for the PR description and the report.
6. `make verify` green. Never skip or weaken a check. Re-run only what failed while fixing.

## Test levels

Logic: T1-T2. Flow change: a T4 scenario. Deploy, runner or egress: T6 escape tests. A fixed bug gets a regression test. Tests never call a real model.

## The PR

Conventional Commit title. Description: what and why (3-5 lines), design section, test levels, risks and rollback, open questions, the `Self-audit` block, and a basis line (Checked / Inferred / Confidence, see AGENTS.md "Evidence"). Your final report to the owner ends with the same line; say what you ran and read, not what you remember. Draft until `make verify` is green. Gated paths (`.github/`, `deploy/`, `schemas/`, `mise.toml`, `mise.lock`, `Makefile`, `AGENTS.md`, `CLAUDE.md`, dependencies) get the label `needs-owner-review` for the morning digest and are merged on green after the independent review. Only secrets, new egress hosts, money, irreversible steps and weakened protections wait for the owner.

## Independent review

A PR that changes code is reviewed by an agent that did not write it before the merge (owner's rule of 2026-11-25, AGENTS.md). Run a fresh subagent with `/code-review` at high effort on the PR diff; do not review your own change in the session that wrote it. Fix every confirmed bug and add a regression test for it, then run the checks again; the findings that you reject get a one-line reason in the PR. Put `Reviewed by: (reviewer), N findings, M fixed` in the PR description. Docs-only PRs skip this step.

## Merge

AGENTS.md lets a session merge its own PR in every phase once `make verify` and CI are green, the independent review is done and no "Ask before you do it" item is open (AGENTS.md lists them; gated paths are not among them since 2026-11-25). Batch real owner items in `docs/status.md`.

## Never stop to wait

A session does not end its turn to wait for CI. After the PR is open, poll its checks in the same turn (the check runs of the PR, every minute or so), fix a red check, merge on green and take the next item of `docs/plan.md` at once. Stop only when blocked on the owner (a new account, a paid service, new egress, an irreversible step) or on a usage limit; after a limit, resume when it resets. This is the owner's rule of 2026-11-25.

A thread whose plan items are still open is never marked done. On a limit, call `send_later` for the reset time, write the resume point in `docs/status.md`, then end the turn: nothing else wakes a stopped thread. At the start of a session, if `docs/status.md` names a resume point, take it first.

## Open PRs

An open PR is unfinished work, and the owner wants none left idle (2026-11-26: PRs sat for hours; the cause was a conflict with `main`, which gives no CI, and threads that ended after opening the PR). So:

- Merge `origin/main` into your branch right before you open the PR and again whenever `main` moves. Conflicts pile up in the shared files `docs/status.md`, `docs/decisions.md`, `docs/plan.md`, `docs/design.html` (hashes and `PLAN`) and `AGENTS.md`; resolve them by keeping both sides and re-running `scripts/repo-check.sh`. ADR numbers go to whoever merges first: renumber yours.
- Do not end the thread while its PR is open. Merge it on green, or close it with a reason. If you are blocked on the owner, say so in the PR and in `docs/status.md`.
- At the start of a session, list the open PRs. Finish your own. For another session's PR that is idle for over 2 hours, ask its thread (a PR comment or a message) and never push to its branch; if the thread has ended or stays silent for 30 minutes, open a PR of your own with its commits that supersedes it, and close the old one with a link. Never delete a branch with unmerged work without the owner's word.
