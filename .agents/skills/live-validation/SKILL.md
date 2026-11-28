---
name: live-validation
description: Run a live validation or live check on the owner's machine (the Mac thread) without wasting the owner's clicks or time. Use before you ask the owner for any click, setting or wait, and when the owner tells you how test runs should work.
---

# Live validation

Rules for the session that runs live checks on the owner's machine (`docs/plan/mvp-validation/README.md`, `docs/plan/live-checks.md`). They come from the audit of 2026-11-28, where the owner asked four times why he had to click.

## Before you ask the owner for a click

Ask only when the click itself is what the check tests, and that gate has not yet passed live. Once a gate has passed (with a task id and a commit as evidence), later rounds do not need the owner for it.

| Step | Who does it |
|---|---|
| Cancel a stuck test task, answer Lead, send an idea, approve a spec after the Slack button has passed live | you, or the coordinator for Slack posts from the owner's account; record "from the terminal" |
| PR Approve or Merge on GitHub in a sandbox tenant, after both paths have passed live | nobody: the sandbox tenant runs without required review (`docs/decisions.md`, "Taken", 2026-11-28) |
| The first live pass of a human gate (spec Approve, PR Approve, PR Merge) | the owner, one click each, asked in one batch before the run |

Never press a button under the owner's account and never promise to. If a click can be avoided only by a setting, name the setting and what it weakens in the same message.

## When the owner tells you how test runs work

An instruction such as "turn review off in the test project" or "approve specs yourself" stands until the owner revokes it in words. Do not tie it to one stage and do not undo it on your own, even if you offered to.

In the same turn:

1. Apply it, backing up the config first.
2. Record it: ask a cloud thread, through the coordinator, to add it to `docs/decisions.md` under "Taken", with the owner's message time.
3. Say in one line what it changes and what stays (the factory's own tenant keeps its review).

## Messages to the owner

- No durations or ETAs for work that has not happened ("in 15–20 minutes"). Give only clock times that already happened, in Zurich time.
- Check before you promise. If a check blocks a step you announced, correct that message at once.
- One message per state change: what changed, what the owner must do (if anything) with a direct link, and nothing else.

## Rounds

- Run every stage that does not depend on a failed one, and stop at the end of the round (`docs/plan/mvp-validation/S3-round-rules.md`).
- A fix found by a round goes to the coordinator with the evidence. After it merges, upgrade with `deploy/upgrade.sh` and rerun only from the first stage it touches.
- Record each result with its evidence (task id, commit, log line) in the entry it covers, as it happens.
