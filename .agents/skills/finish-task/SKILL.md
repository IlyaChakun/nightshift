---
name: finish-task
description: Wrap up a task on this repo: update plan and status, run make verify, open a draft PR with the required description, mark it ready and merge it under the merge rule. Use before every push and PR.
---

# Finish a task

Start with the approved item or the next item in the handoff. Read `docs/status.md`, `docs/plan.md` and the relevant phase plan. On continuation, load the current item, existing evidence and changes since the recorded SHA. Work outside the plan needs the owner's OK and a plan update first.

## Before the PR

1. One task, one branch `feat/<topic>`, `fix/<topic>` or `docs/<topic>`; never push to `main`. Keep the diff under about 400 lines.
2. Run `scripts/pr-gate.sh preflight` before starting a GitHub batch. If the active account has read access, `scripts/pr-gate.sh --git-auth preflight` uses the existing HTTPS origin credential for this process.
3. Run the targeted regression and the cheap checks for the diff. Use `make tools fmt` for Go, `make gen-check` for generated code, and `make repo-check` after staging plan or design edits. Start independent review while the focused checks run. Resolve the findings, then run a full `make verify` on the final tree before push.
4. If you own the phase README, update its Status and item. Otherwise update only the item file. Edit `docs/status.md` for cross-phase facts only. Update the appropriate operator docs when an operator action changes.
5. Run `self-audit`. If a real-stack check is owed, keep the item at `State: open` and add one file under `docs/live-checks/` with its scope, run, expected result and `Status: pending`. Report "merged, live check pending".

## Test levels

Logic: T1-T2. Flow change: a T4 scenario. Deploy, runner or egress: T6 escape tests. A fixed bug gets a regression test. Tests never call a real model.

## The PR

Conventional Commit title. Description: what and why (3-5 lines), design section, test levels, risks and rollback, open questions, the `Self-audit` block, and a basis line (Checked / Inferred / Confidence, see AGENTS.md "Evidence"). Your final report to the owner ends with the same line; say what you ran and read, not what you remember. Draft until `make verify` is green. Gated paths (`.github/`, `deploy/`, `schemas/`, `mise.toml`, `mise.lock`, `Makefile`, `AGENTS.md`, `CLAUDE.md`, dependencies) get the label `needs-owner-review` for the morning digest and are merged on green after the independent review. Only secrets, new egress hosts, money, irreversible steps and weakened protections wait for the owner.

## Independent review

A PR that changes code is reviewed by an agent that did not write it before the merge (owner's rule of 2026-11-25, AGENTS.md). Run a fresh subagent with `/code-review` at high effort on the PR diff; do not review your own change in the session that wrote it. Fix every confirmed bug and add a regression test for it, then run the checks again; the findings that you reject get a one-line reason in the PR. Put `Reviewed by: (reviewer), N findings, M fixed` in the PR description. Docs-only PRs skip this step.

## Merge

AGENTS.md lets a session merge its own PR in every phase once `make verify` and CI are green, the independent review is done and no "Ask before you do it" item is open (AGENTS.md lists them; gated paths are not among them since 2026-11-25). Batch real owner items in `docs/decisions.md`.

Use the autonomous merge gate skill before any merge. After local verification and independent review on the PR head, run `scripts/pr-gate.sh wait <PR> <verified-head>` to check readiness. When authorized to merge under AGENTS.md, run `scripts/pr-gate.sh merge <PR> <verified-head>` directly. It waits for checks, checks the exact head and latest green main, and watches the merged commit. Both modes accept `--git-auth` before the subcommand. If main moves, integrate it, review the effect, verify the new head and retry.

A session keeps ownership while the helper waits. Check its running session at bounded intervals. If a check fails, diagnose it and finish the PR under the merge rule.

When stopping on a usage limit, record the resume point before stopping. Use an available authorized wake mechanism when the reset is known. If there is none, record that the work needs a new turn. A pending PR or live check is still open work.

## Thread size and handoff

Triggers: `docs/plan.md`, "How threads are run". Run `scripts/thread-size.sh` after each merge and before a new item; exit 1 means a trigger fired. A new stream, phase or usage limit is one too; new work the owner asks for here goes to a new thread unless the owner says otherwise. Then end the thread:

1. Merge or close every PR of yours, each with a reason.
2. Rewrite the "Status" block of your phase file: last done, next, open, live checks waiting.
3. Hand off at most 10 lines: next item, base SHA, owned files, branch or PR, test evidence, open risks, links. Say "handed off", not "done", while plan items are open.

## Token use

Every tool call re-reads the whole thread context; re-reading is most of the spend (`docs/research/token-economy.md`). So:

- Wait for CI with one blocking call (`scripts/pr-gate.sh wait`, a PR subscription), never repeated short checks. Every wake re-reads the context: open the PR ready once `make verify` is green, and unsubscribe after the merge.
- Read the section you need: `grep -n '^## ' docs/design.md docs/design/*.md`, then that range, never a whole design file.
- Send independent reads and searches in one message.
- Keep command output short: `2>&1 | tail -n 40`, quiet flags, failures only. Give a big read (many files, many records) to a subagent on a cheap model that returns a summary.

## Open PRs

An open PR is unfinished work; the owner wants none idle (2026-11-26: a conflict with `main` gives no CI, and threads ended after opening). So:

- Merge `origin/main` into your branch right before you open the PR and again whenever `main` moves. Conflicts pile up in the shared files `docs/status.md`, `docs/decisions.md`, `docs/design.html` (hashes) and `AGENTS.md`; plan files are owned per phase and rarely clash; resolve them by keeping both sides and re-running `scripts/repo-check.sh`. ADR numbers go to whoever merges first: renumber yours.
- Do not end the thread while its PR is open. Merge it on green, or close it with a reason. If you are blocked on the owner, say so in the PR and in `docs/decisions.md`.
- At the start of a session, list the open PRs. Finish your own. For another session's PR that is idle for over 2 hours, ask its thread (a PR comment or a message) and never push to its branch; if the thread has ended or stays silent for 30 minutes, open a PR of your own with its commits that supersedes it, and close the old one with a link. Never delete a branch with unmerged work without the owner's word.
