---
status: accepted
date: 2026-11-26
---

# Require evidence before automatic review and release

## Context and Problem Statement

A tenant can operate without a Verifier. The conductor also reads release capabilities and quiet hours at runtime. Each decision needs an explicit outcome that survives a restart. Source: [audit #332](https://github.com/IlyaChakun/nightshift/pull/332).

## Considered Options

- Require an authorized reviewer when verification is unavailable and retry a release decision when its inputs are unknown.
- Require every tenant to configure a Verifier and reject its configuration until it does.

## Decision Outcome

Use a human review for a change without a passing Verifier result. Record an unavailable Verifier in the task transition. Automatic review decisions require a consumed passing result for the task’s recorded head and the corresponding review transition.

Prefer waiting over releasing late when scheduling inputs are unknown. Read the release capability before applying the schedule. A declared target waits for its adapter, recipe and configured deployer. A repository with no release target can merge at any hour.

An approval covers the patch it was given to. The train journals a rebase as done only when it started in the same run and kept the patch-id. Any other rebase sends the task back to human review at the rebased head, so a crash between the push and the journal write cannot merge on a stale approval. A head on the code host is offered for that review only when it replays the recorded commits on the base branch; any other push needs reconciliation.

### Consequences

An operator may need to restore access to a repository or correct its schedule before the train resumes. After recovery, the next tick retries. A reviewer can approve a specific change for a tenant with no Verifier.

### Confirmation

`TestRequiredReviewAwaitsEvidence` checks the held states. `TestReleaseAwaitsKnownPolicyAndRetries` checks recovery. `TestAPushedRebaseWithoutJournalGoesToReviewAtTheHostHead` and the `internal/train` tests check that a changed or resumed rebase waits for a fresh approval. The [live check](../live-checks/A01a-required-checks.md) covers the operator-visible results.
