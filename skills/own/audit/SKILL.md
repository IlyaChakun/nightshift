---
name: audit
description: Audit a slow, flaky or wasteful part of the factory (CI, tests, local checks, agent loops, cost) by measuring it, finding the root cause and proposing the cheapest fix. Use when something waits, reruns or fails for a reason that is not the change itself.
---

# Audit

Slow or flaky processes are defects. Do not wait them out and do not work around them: measure, explain, fix or propose.

## When to start

- A CI run takes longer than about 3 minutes, or a PR needs a second CI round.
- A check fails in CI but passes locally, or the other way round.
- A test is flaky, a tool is re-downloaded on every run, or an agent loops on the same step.
- You notice yourself waiting. Waiting is the trigger.

## Steps

1. **Measure.** Collect real numbers, not impressions: `gh run list` and `gh run view <id> --json jobs` for per-job and per-step time, `go test -json` or `-v` for per-package time, a rerun count per PR. Compare the last 20-40 runs.
2. **Find the biggest cost.** Rank steps by share of wall time. Work on the top one; ignore the rest.
3. **Find the cause.** Ask why until it is a fact you can point to: a missing cache, a serial step that can run in parallel, a tool version that differs from the pin, a test that sleeps, a check that only runs in CI. Reproduce locally when you can.
4. **Choose the cheapest fix.** In order: remove the work, run it in parallel, cache it, run it later (nightly or merge queue), move it earlier (local check, pre-push). Never skip or weaken a check to save time.
5. **Fix it or propose it.** A small fix goes in a separate PR with before and after numbers. A larger or gated change (see AGENTS.md "Ask before you do it") goes to `docs/status.md` as a question with options and a recommendation.
6. **Keep the result.** If the cause was a missing rule, add a linter, check or test; text rules come last. Record durable lessons in `AGENTS.md` or `LEARNINGS.md` only when no tool can enforce them.

## Report

Lead with the verdict and numbers before and after, then the cause, the change and the trade-off. Mark anything unmeasured as unverified.

## Factory-wide audit

Run the steps above over each area in turn and write one finding per root cause: CI (job and step time, rerun rate), local `make verify` parity with CI, test suite (slowest packages, flaky tests), tool pins and lockfiles, worker image build time, agent runs (tokens, retries, time per role), and book queries. Order findings by wall time saved per hour of work.
