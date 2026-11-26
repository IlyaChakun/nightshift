---
status: accepted
date: 2026-11-26
---

# Scope worker state to one attempt of a job

## Context and Problem Statement

The jobs API hands a job to a new attempt when a lease runs out, but the job id stays the same. The worker kept the checkout under the task and the patch under the job id, and only logged a failed heartbeat. An attempt that lost its lease could keep working and change what the newer attempt reads or delivers. Source: [audit #335](https://github.com/IlyaChakun/nightshift/pull/335).

## Considered Options

- Name the checkout and the artifacts by attempt, and stop an attempt whose lease is gone.
- Run each job in its own container, so attempts never share a filesystem.
- Lock the task directory for one attempt at a time.

## Decision Outcome

Chosen: the first option. It needs no new service and keeps one worker container (design section 17, ADR 0015). An attempt is named by a hash of its job token. The checkout is `<work>/<tenant>/<task>-<attempt>` and the patch `<artifacts>/<job>/<attempt>/patch.diff`; the recorded artifact path names the attempt, and the core keeps checking its SHA-256. A refused heartbeat, or a whole lease without a successful one, cancels the attempt's context, and no turn starts after that. The worker removes checkouts a crashed run left when it starts.

### Consequences

A second worker replica on the same `/work` volume loses its live checkouts when the other one starts; its jobs are retried (plan M3 keeps replicas open). Patches recorded under the bare name by older workers stay readable.

### Confirmation

`TestAnAttemptThatLostItsLeaseStopsItsTurn`, `TestNoTurnStartsAfterTheLeaseRanOut`, `TestAttemptsOfOneJobKeepTheirOwnCheckoutAndPatch`, `TestTheWorkerRemovesLeftoverCheckoutsWhenItStarts`, `TestHeartbeatReportsALostLease`, `TestAStaleAttemptCannotReplaceTheCompletedPatch` and `TestIsPatchPath`. The [live check](../live-checks/A04-attempt-isolation.md) runs it on the stable worker.
