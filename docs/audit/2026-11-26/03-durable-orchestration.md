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

## Decision brief — transactional result application, 2026-11-26

**Recommendation:** retain Go/PostgreSQL/River and ADR 0018. Fix the concrete mandatory-event failure first; then make DB-local result consumption atomic and test crash boundaries. Existing lease tokens, CAS, outbox and train reconciliation are valuable foundations. Do not introduce another workflow engine to solve a local transaction problem.

### Static dispositions at the pinned base

| Finding | Evidence and significance |
| --- | --- |
| A03.1 confirmed error-handling gap | [appendEvent](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/build.go#L140-L147) logs and drops the write error. [consumeVerdict](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/verify.go#L132-L142) records a raised risk through it before consuming and moving. [needsHuman](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/gate.go#L114-L125) reads that event. A transient isolated write failure can therefore remove the persisted reason to hold a task. Actual production effect depends on other policy controls. |
| Additional retry-control gap | [sendBack](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/build.go#L149-L165) counts prior return events but writes the new return with the same best-effort helper; a failed append can undercount attempts. Conversely a successful append followed by a failure before consumption can count an attempt twice on retry. Independent budget caps may still apply. |
| A03.2 confirmed separate DB commits | [consumeVerdict](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/verify.go#L102-L142) stores findings, risk, consumption and move separately. Its comment explicitly describes rerunning review after a crash. [escalate](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/build.go#L54-L62) consumes before moving to escalation. [Builder success](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/build.go#L117-L137) uses move-first reconciliation instead. These are different recovery contracts; not all are defects. |
| Existing atomic transition | [Mover.Apply and record](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/flow/move.go#L38-L78) already combine state CAS and move event in a transaction; [record](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/flow/move.go#L100-L119) also cancels outstanding jobs on cancellation. Reuse this logic. |
| Existing lease fencing | [jobs.sql](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/book/queries/jobs.sql#L20-L68) rotates lease token on claim; completion and heartbeat require a current unexpired token. [CancelTaskJobs/ConsumeJob](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/book/queries/jobs.sql#L87-L101) revoke tokens and mark consumption once. End-to-end coverage of every artifact and result write still needs testing; do not claim fencing is missing wholesale. |
| A03.3 assurance gap | [side-effects test](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/sideeffects_test.go#L14-L77) checks an AST inventory and a reason string, not every actual external behavior. [Outbox](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/outbox.go#L67-L117) deduplicates enqueue and delivers a batch inside a DB transaction; send followed by failed commit still requires adapter idempotency. |

### Relevant primary references

- [River transactional enqueueing](https://riverqueue.com/docs/transactional-enqueueing), read 2026-11-26: application changes and enqueue can commit in one database transaction. Nightshift agent jobs already use its own book.jobs SQL (not automatically River jobs); [River wiring](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/river.go#L45-L82) includes outbox flushing. Use the existing book queries in the same transaction; avoid a mistaken migration to River APIs for every agent job.
- [Transactional outbox pattern](https://microservices.io/patterns/data/transactional-outbox.html), read 2026-11-26: persist intent atomically, then relay; duplicate delivery remains possible after relay failure. Use provider-supported idempotency or reconciliation; distinguish one logical effect from one network call.
- ADR 0018 and existing durable-execution research remain canonical; revisit only if operational evidence overturns their conclusion.

### Options and concrete approach

| Option | Trade-off | Decision |
| --- | --- | --- |
| Return mandatory append errors | Small urgent fix, but does not eliminate separate commit windows | P0 containment |
| DB-local transactional result application | Stronger atomicity with no new infrastructure; requires a reusable transaction-aware state transition API | Recommended |
| Persist intermediate states and reconcile all work | Appropriate for GitHub/deployment; additional state and recovery complexity for DB-local work | Use across external boundaries |
| Temporal/another orchestrator | Migration, runtime and operating cost; still needs idempotent external activities and domain authorization | Defer under ADR 0018 |

Suggested transaction: lock or CAS the expected task generation and verify the result belongs to the intended job, role and candidate; claim the unconsumed terminal result, persist unique findings and mandatory risk/return facts, perform the existing transition and append its event, enqueue dependent DB-local work or outbox intent where applicable, commit. If any step fails, roll back all DB-local effects. A result already applied returns no-op; do not simply ignore a lost consumption claim and continue. Avoid holding transactions across model, git or network operations. Store actual usage of each physical attempt separately from logical transition counters.

Existing Mover.Apply owns its transaction; propose a transaction-scoped helper rather than nesting independent commits or duplicating transition rules. Existing outbox and merge/deploy journals remain responsible for uncertain external outcomes; a successful remote action with lost reply must reconcile against the actual remote identity and revision. The outbox network call under the current DB transaction is a latency/lock-duration risk to measure, not automatically a reason to replace its relay.

### Minimal fault matrix

| Injected fault | Required observation |
| --- | --- |
| Mandatory raised-risk insert fails | No dependent progress; retry retains original verdict and risk |
| Crash after consumption before move / after move before acknowledgement | One logical result, no lost authorization or duplicate return; bounded recovery |
| Two conductors consume same terminal job | Only one transaction applies the result |
| Lease expires, old completion arrives, cancel races | Old token cannot mutate result, artifacts or budget; no unauthorized next step |
| Remote merge/send succeeds, local acknowledgement fails | Reconcile actual remote state; one logical operation or documented unavoidable duplicate |
| DB unavailable during optional logging | Telemetry failure is visible but does not corrupt authoritative state |

Start with real Postgres integration and deterministic fault injection; reuse existing T2/T3/T4/T5 suites and the side-effect inventory. Add live adapter contract checks for the small number of irreversible or externally idempotent operations. Verify rollback boundaries, restart liveness and resource limits, not only final task state.

**Decision gate:** no workflow-engine migration for these findings. Reopen ADR 0018 only after recurring incidents or measured maintenance cost demonstrate a limitation that persists after local atomicity fixes. Coordinate final release authorization with A01 and per-attempt artifact access with A04.

**Checked:** pinned source paths above, design/plan R2 and River/outbox references. **Inferred:** isolated failure consequences, recommended transaction design and priority. **Confidence:** high for static write ordering, medium for runtime impact. **Not verified:** tests, concurrent execution, failure injection, provider idempotency and production exposure.
