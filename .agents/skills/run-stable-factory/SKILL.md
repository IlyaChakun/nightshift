---
name: run-stable-factory
description: Let the factory develop its own repository on a pinned stable instance: start or upgrade it by tag, queue a small task, watch it to release, get an independent review. Use for dogfooding, "give this to the factory", or upgrading factory-stable.
---

# Run the factory on itself

`docs/runbook.md`, "Stable instance", is the only source of the setup, upgrade and rollback steps. Everything goes through `scripts/stable.sh`; never run plain `docker compose` on a release.

## Pick a task

- From a file under `docs/plan/`, or a small doc, test or fix that an open item needs. Under about 100 changed lines, one concern.
- Not what another session or open PR is touching: check `gh pr list` and the open items of the phase files under `docs/plan/` first.
- Not a gated path (`AGENTS.md`, "Ask before you do it") or a `critical` path of `.nightshift/blast-radius.yaml`: the factory rejects protected paths and the rest waits for the owner.
- Checkable in the worker: `.nightshift/project.yaml: verify` (Go build, vet, tests). Tests that need Docker run only in CI.

## Queue and watch

1. `scripts/stable.sh compose exec nightshift nightshift doctor` is green and no task is in `building` or `checks`. One task at a time.
2. `scripts/stable.sh compose exec -T nightshift nightshift task add nightshift "<type>: <what, with the acceptance check>"`. The text is the PR title and Builder's whole task: one line, Conventional Commit style, with the acceptance check, at most 120 characters (the rest is dropped). A task that needs more words goes through the chat channel, where Lead writes a spec.
3. Follow it: `... nightshift status --tenant nightshift`; events with `compose exec -T postgres psql -U postgres -d book -c "SELECT kind, payload FROM book.events WHERE task_id = <id> ORDER BY id"`; logs with `compose logs worker nightshift`. Wait in the same session until it reaches `human_review`, or `released` for a low-risk change.

## Review and release

1. A fresh reviewer that did not write the change reviews the PR diff (a subagent running `/code-review` at high effort for code; a read-through against the code for docs). Check its findings yourself and post them on the PR with who reviewed.
2. A confirmed defect: close the PR and queue a new task that names the defect. Do not send "request changes" until the stable tag gives Builder the review text, and then only for a change Builder can redo from scratch: it starts again from `main`. Do not push to the factory's branch.
3. A `low` risk change (the tenant's `review` policy) merges on green CI by itself: review it after the merge and fix what you find in a new PR. Everything else waits for the owner's Approve on GitHub. Never approve for them unless they ask for that PR in their own words.
4. Check that the task reaches `released` and `main` CI is green.

## When the factory misbehaves

Fix the factory in a normal PR with a regression test (not through the factory), then run `deploy/upgrade.sh` (runbook, "Upgrade"): it checks the commit live and moves the stable tag. Note the defect in `docs/status.md` if it stays open.
