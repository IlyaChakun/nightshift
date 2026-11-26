---
name: audit
description: Audit factory work and improve the workflow when asked. Use for recent PRs, slow or repeated checks, agent context growth, or routine blockers. Measure the problem, verify the cause, and apply authorized fixes through the normal PR checks and review.
---

# Audit

Slow or flaky processes are defects. Do not wait them out and do not work around them: measure, explain, fix or propose.

## When to start

- A CI run takes longer than about 3 minutes, or a PR needs a second CI round.
- A check fails in CI but passes locally, or the other way round.
- A test is flaky, a tool is re-downloaded on every run, or an agent loops on the same step.
- Expected CI waiting uses the helper in `finish-task`. Audit a failed or unusually slow run.

## Review a batch of work

1. Pin the current main SHA and the batch's PRs. Read the handoff and recent logs first. For each relevant PR, compare time to open, time to merge, check reruns and post-merge results. Read exact usage records when available. Keep cached input, uncached input and output separate. Report unknown usage plainly.
2. Compare actual decisions and recovery steps with the skill or script that governs them. Reproduce the cheapest failure first. Separate a baseline problem reproduced on main from one caused by the current change.
3. If asked for an audit, report ranked changes and how to verify them. If asked to improve or fix the workflow, take the first confirmed authorized item. Use focused checks, independent review, full verification and the merge procedure in `finish-task`. Re-run the original measurement under comparable conditions and record both runs.
4. Continue through the authorized findings. Keep unresolved live checks and follow-ups in the plan. At a handoff, link the measurements and next item in the short brief from `finish-task`.

## Steps

1. **Measure.** Collect real numbers, not impressions: `gh run list` and `gh run view <id> --json jobs` for per-job and per-step time, `go test -json` or `-v` for per-package time, a rerun count per PR. Compare the last 20-40 runs.
2. **Find the biggest cost.** Rank steps by share of wall time. Work on the top one; ignore the rest.
3. **Find the cause.** Ask why until it is a fact you can point to: a missing cache, a serial step that can run in parallel, a tool version that differs from the pin, a test that sleeps, a check that only runs in CI. Reproduce locally when you can.
4. **Choose the cheapest fix.** In order: remove the work, run it in parallel, cache it, run it later (nightly or merge queue), move it earlier (local check, pre-push). Never skip or weaken a check to save time.
5. **Follow the requested mode.** Report audit-only findings. When fixes are authorized, use separate reviewable PRs with before and after evidence. Follow `finish-task` and record an owner decision only when AGENTS.md requires one.
6. **Keep the result.** If the cause was a missing rule, add a linter, check or test; text rules come last. Record durable lessons in `AGENTS.md` or `LEARNINGS.md` only when no tool can enforce them.

## Report

Lead with the verdict and numbers before and after, then the cause, the change and the trade-off. Mark anything unmeasured as unverified.

## Factory-wide audit

Run the steps above over each area in turn and write one finding per root cause: CI (job and step time, rerun rate), local `make verify` parity with CI, test suite (slowest packages, flaky tests), tool pins and lockfiles, worker image build time, agent runs (tokens, retries, time per role), and book queries. Order findings by wall time saved per hour of work.
