---
status: accepted
date: 2026-11-26
---

# Persist dispatch decisions before consuming results

## Context and Problem Statement

The Builder return limit and the human review policy read facts from the book. A failed write could lose a return or a raised Verifier risk while the conductor continued. Source: [audit #334](https://github.com/IlyaChakun/nightshift/pull/334).

## Considered Options

- Keep event writes best effort and use logs to investigate missing entries.
- Commit each required event with the job consumption mark and report storage failures.

## Decision Outcome

Commit the Builder return or the raised review risk and the job consumption mark in one Postgres transaction. A failed write leaves the same result available for retry. A job that has already been consumed cannot record a second return. Optional events still use the best-effort path.

The next dispatch tick can enqueue Builder after a committed return. Verifier findings, raised risk, job consumption and the task transition commit together. A failed write keeps the same result available and reports the first storage error. A concurrent tick that sees a superseded review cannot replay its result. The task counts only applied returns to Builder against its review-return budget.

### Confirmation

`TestBuilderReturnRetriesItsRecord` and `TestVerifierRaisedRadiusHoldsForAPerson` use Postgres constraints to reject each required event and check recovery.

`TestVerifierVerdictRollsBackAndReplaysOnce` rejects writes before and after the task update, then retries with two conductors. `TestVerifierBudgetIgnoresPriorPassAndStaleReturn` checks the return count.
