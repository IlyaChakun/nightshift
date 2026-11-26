# NS-A03 — Atomicity, durable orchestration and recovery

**Priority:** P0 for authorization-critical event loss; P1 for job result consumption and crash recovery.  
**Status:** candidate code findings; reproduce before fixing.

## A03.1 — Critical domain events may be lost without blocking progress

**Earlier observation:** [build.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/build.go#L133-L156), [verify.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/verify.go#L98-L138), [gate.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/gate.go#L106-L119). `appendEvent()` appeared to log a write error without propagating it; verifier risk decisions appeared to be written as events later consulted by `needsHuman()`.

If this is still true, a failed event write could remove an escalation decision while execution continues. Verify whether other authoritative persisted fields or fail-closed checks eliminate that path. This is not a reproduced incident.

**Invariant:** authorization, budget, risk and state decisions must be durably recorded before any dependent transition or external action. Optional telemetry failure must not invalidate business state.

**Options:** propagate errors for mandatory events (small fix but callers may mishandle retries); separate critical domain events from best-effort telemetry and persist them in the same transaction as their state changes (stronger invariant, schema/API changes); store policy decision as authoritative state with a corresponding event (easier reads, duplication must be transactionally maintained).

**Acceptance:** inject write failure at each relevant decision, assert no unauthorized transition or side effect, retry safely, prove escalation survives restart, and distinguish optional logging failure from mandatory persistence failure.

## A03.2 — Consuming completed jobs may span multiple commits

**Earlier observation:** [move.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/flow/move.go), [verify.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/verify.go#L98-L138), [build.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/build.go#L50-L58). State CAS and transition event appeared atomic; result consumption, findings and phase transition appeared to use separate operations. The reviewed code described a crash window that could repeat a verifier run. Repeat execution may be intentionally safe; assess convergence counters, cost, stale results and actual reconciliation.

**Invariant:** a logical result is applied at most once to authoritative state; work can be retried, but a stale worker or duplicate delivery cannot corrupt progress, charge a logical budget twice unintentionally, or authorize a later attempt.

**Options and trade-offs:**
- Single PostgreSQL transaction for DB-local result, findings, policy facts, CAS transition, consumption marker and next job/outbox insert (where River and existing APIs support transactional enqueue). Strong local atomicity; keep transactions short and do not call external systems inside.
- Idempotent multi-step reconciliation with unique operation IDs, explicit intermediate states and retry-safe compensation. Useful for cross-boundary actions; more state and crash windows to reason about.
- Lease/attempt fencing tokens checked on every state-changing write; solves stale ownership, not external exactly-once execution. External effects require provider idempotency or read-after-uncertain-outcome reconciliation.

**Suggested approach:** inventory each path; use one transaction for local effects, unique attempt/result identities and CAS/fencing where needed, existing outbox for external effects. Keep external effects at-least-once with explicit reconciliation. Do not claim exactly-once external execution without proof.

## A03.3 — Recovery assurance and Temporal decision

The earlier review identified [sideeffects_test.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/sideeffects_test.go) as an AST-based guard for documented retry safety, not behavioral proof. Review existing recovery drills, transactional outbox and merge journal before adding redundant machinery.

Test process death before/after DB commit, duplicate job completion, concurrent conductors, expired lease with resumed old worker, cancellation during retry, GitHub success followed by lost response, provider timeout and DB unavailability. Assert invariants and bounded recovery, not just eventual happy-path success. Model-based tests and deterministic failure injection can cover permutations that scenario examples miss.

**Temporal:** the earlier review reported ADR 0018 and [durable-execution.md](https://github.com/IlyaChakun/nightshift/blob/main/docs/research/durable-execution.md) favoring existing Go/PostgreSQL/River. Respect that decision unless new evidence changes it. Measure recovery failure rate, custom orchestration code/maintenance effort, recurring incidents, workflow complexity and operational costs. Revisit Temporal/DBOS/Restate only if a representative migration spike demonstrates a material benefit after accounting for migration and operating costs. Temporal would not eliminate external idempotency or release policy obligations.

## Acceptance criteria

A fault matrix records observed outcome at every relevant crash boundary. Confirm critical events cannot be lost, duplicate completion cannot mutate authoritative state twice, stale workers cannot advance tasks, outbox recovery avoids duplicate logical actions where supported, and retries respect budget/cancellation. Existing CI and targeted integration tests pass. Document any unavoidable duplicate external calls and how ambiguity is reconciled.

## Follow-up source check and retention

This dated external review is a candidate finding record, not the living design or a verified defect register. Preserve the original findings and append a disposition or link to implementation evidence; do not delete this record merely because an issue is resolved. Superseding or relocating it requires an explicit owner decision.

**Checked:** pinned Nightshift base [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26. At the pinned base, `internal/dispatch/verify.go` lines 102–133 retains consume-before-move and raised-risk event recording; `internal/dispatch/build.go` lines 140–142 retains appendEvent. Test the complete transaction and crash semantics before declaring a defect. ADR 0018 remains the no-Temporal decision.

**Inferred:** proposed priorities, failure modes and remedies in this document; no tests or live runs performed. **Confidence:** medium for the risk assessment, not a confirmed incident.
