# Nightshift — architecture and reliability audit

**Review date:** 2026-11-26  
**Status:** proposed engineering backlog; not a verified defect register  
**Scope:** Go orchestration, River/PostgreSQL durability, agent execution, verification, release authority, product-level autonomy and learning loops.

This audit consolidates the earlier external review of Nightshift's documentation and selected source files. It is intentionally a documentation-only proposal. The source was not cloned, tests were not executed, GitHub protection settings were not inspected, and a fresh fetch failed when preparing this package. The exact reviewed commit is unknown. All links point to the mutable `main` branch and are navigation hints, not immutable evidence. Before implementation, verify every finding against the current HEAD and existing ADRs, tests, backlog and operational configuration. Do not assume a candidate finding is still present.

## Findings and suggested sequencing

| ID | Area | Suggested priority | Document |
|---|---|---|---|
| NS-A01 | Verification, release authorization, evidence | P0/P1 | [Verification and release integrity](01-verification-release.md) |
| NS-A02 | Self-modification and policy trust boundary | P0 | [Trusted self-improvement](02-trust-self-improvement.md) |
| NS-A03 | Atomicity, durability, retries and recovery | P0/P1 | [Durable orchestration](03-durable-orchestration.md) |
| NS-A04 | Sandbox and agent isolation | P1; P0 if secrets or hidden tests are exposed | [Execution isolation](04-execution-isolation.md) |
| NS-A05 | Product-level completion and cross-task integration | P2 | [Product orchestration](05-product-orchestration.md) |
| NS-A06 | Convergence, context and corrective strategies | P2 | [Agent convergence](06-agent-convergence.md) |
| NS-A07 | Live proof, evaluation and continuous improvement | P1/P2 | [Operational validation](07-operational-validation.md) |

P0 means a candidate release/security invariant that must be resolved or explicitly disproved before unrestricted autonomous production operation. P1 means reliability or assurance work before scaling; P2 improves effectiveness and maturity. These are proposed priorities, not severity ratings based on reproduced incidents.

## Instructions for implementation agents

1. Read `AGENTS.md`, current architecture, ADRs, research, status and relevant tests. Record the current commit SHA. Respect existing repository conventions and ownership rules.
2. For each finding, record `confirmed`, `partially confirmed`, `already fixed`, `not applicable`, or `unverified`, with file/line references at that commit. Where possible, reproduce the failure in a focused test before changing code. Do not invent a failing test result.
3. Define the invariant and minimum fix. Prefer existing Go/PostgreSQL/River mechanisms over new frameworks. Do not implement every option in these documents; select the simplest one that demonstrably meets the invariant. Explain material trade-offs in an ADR when appropriate.
4. Create small, reviewable changes grouped by failure mode. Do not weaken tests, acceptance gates, approval rules, isolation or resource limits to make changes pass. Keep production credentials and hidden evaluation inputs out of logs and fixtures.
5. Verify each change using focused regression tests, relevant integration tests and the repository's normal CI checks. Test crash boundaries and concurrency explicitly. For changes to gate logic, include negative tests that demonstrate the gate cannot be bypassed.
6. Report evidence: commit, commands, actual results, known limitations, residual risks, and follow-up items. If a finding is already fixed, document proof rather than creating redundant code.

## Dependency and rollout guidance

- A01/A02/A03 share the same security and transactional invariants; agree on authoritative release and policy evidence before independently editing those paths.
- A04 must establish isolation for independent verification before live hidden-scenario evaluation in A07.
- A05/A06 can be staged after correctness invariants and a baseline live run. Start A07's measurement baseline early; expand only once the relevant safety gates are demonstrated.
- Revisit Temporal only if measured orchestration maintenance/recovery costs justify it; no framework migration is implied by this audit.

## Review exit criteria

Every P0 candidate is confirmed and fixed with evidence, disproved with evidence, or explicitly blocked from production operation. P1 work has a tested fix or a documented owner-approved mitigation. P2 work has a baseline and prioritized implementation plan. No statement of production readiness is inferred from fake-only or stub-only tests.

## Retention and disposition

Keep this dated owner-requested external review as a historical source, not an alternative living design. Preserve the original findings and append links to decisions, implementation PRs, regression tests and current disposition. Do not remove or silently rewrite the record during plan/status cleanup; request explicit owner approval before deletion or relocation. This is a documentation convention, not a technical permission boundary. The canonical design remains `docs/design.md`; reconcile durable research with `docs/prior-art.md` and `docs/research/README.md`.

**Checked:** repository documentation rules at [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26. **Inferred:** retention convention and sequencing. **Confidence:** high for the intended organization; no local checks performed.

## Consolidated disposition and verification order (2026-11-26)

This supplement supersedes the initial unknown-base statement for the later pinned review only. The original selective audit remains historical. Connector read returned main [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15) and 21 open draft research/audit PRs, #332–352. The completed Deep Research report's public-page impression of a newer main and six open PRs was not confirmed by the connector. Re-pin before implementation. See the [research inventory](../../research/nightshift-evolution-2026-11-26/README.md) for verified branch heads and source transfer status.

### Evidence vocabulary

- **confirmed-static:** specific observed code or configuration behavior at a pinned revision.
- **helper-reproduced:** isolated mechanism reproduced, not an integrated Nightshift run.
- **integration-verified:** actual repository integration test with command, revision and output.
- **live-verified:** actual authorized deployment/harness/environment with independent evidence.
- **inferred:** plausible consequence from source, not a reproduced incident.
- **proposal:** suggested capability, design or experiment, not approved implementation.
- **not-verified:** absent or inaccessible evidence; not a pass or proof of absence.

Separate observation from consequence and disposition. Earlier helper reproductions were reported in individual briefs; this consolidation does not rerun them. No new integrated/live verification is asserted. Preserve uncertainty and negative results; never treat a reference between proposals as proof.

### Shared implementation work packages

| Work package | Research inputs | Priority / dependency | Minimum disposition |
| --- | --- | --- | --- |
| Authority and exact evidence | A01, A02, A03, #347, #348 | P0, agree first | Explicit required-check outcomes, artifact/spec/policy/actor binding, commit-time version fence and independent promotion. Unknown blocks mandatory action. |
| Durable transitions and external side effects | A03, A01, #347 | P0/P1, shared authority contract | Atomic local result/transition/job/outbox writes, idempotent action journal, lost-ack reconciliation and stale lease fencing. |
| Execution and evaluator isolation | A04, A02, #350, #351 | P1; P0 if unauthorized secret/holdout access confirmed | Per-attempt boundaries, trusted evaluator, negative cross-tenant tests, no widening trust domain. |
| Finding/closure and baseline integrity | A06, #349, #350 | P1 | Reproduce actual decoder/duplicate and fixed-on-enqueue behavior; test severity escalation through full gate; immutable evaluation identities and trial history. |
| Live proof | A07, #342, #346 | Start baseline early; release only after safety proof | Approved real sandbox, failure recovery, independently verified deployment/rollback, repeated representative cohorts. |
| Integrated objective delivery | A05, #347, #337, #348 | P1 after exact candidate evidence | Preserve parent spec and membership, criterion traceability, combined candidate and bounded repair. |
| Measured efficiency | #344, #345, #348, #349 | P1/P2 after reliable accounting | Advisory impact brief, all-attempt cost, attention and debt measures; compare to simpler baseline. |
| Product and operations feedback | #340, #346, #342 | P1/P2 after real delivery | Owner charter, scoped evidence, read-only incident triage, verified recovery and user outcome. |
| Controlled self-improvement | A02, A06, #350 | P2 after reliable evidence and evaluator | Protected baseline/candidate experiment, external promotion/rollback, no self-authorized gate changes. |
| Optional reuse and extension | #351, #352 | P2/P3 conditional | Tenant-local first; reviewed synthetic artifacts only after isolation; existing presets before manifest; defer if not justified. |

### Findings to verify first

1. A01: required verification absent/unsupported; stale or incomplete acceptance evidence; approvals kept versus actual merge authority. Inspect policy and final boundary.
2. A02: candidate cannot promote its own code, alter evaluator or inherit trusted secrets; independently verify rollback and host rules.
3. A03: consumption and movement crash windows, best-effort required events, lost external acknowledgement; reproduce transactional and concurrent failures.
4. A04: shared worker resources and attempt identity; actual cross-tenant/hidden-test access and stale process overlap remain unverified.
5. #348: displayed approval version vs current version and CAS race; #347: base movement during CI and concurrent train ownership. Validate together with A01/A03.
6. A05: expansion replaces parent task and omits full spec after decomposition, explicit aggregate acceptance not found in reviewed path. Test a multi-task shared-invariant failure.
7. A06: snake_case decoder mismatch (helper-reproduced as reported), double ingestion (inferred), fixed-on-enqueue (static); #349: equal-fingerprint severity escalation (helper-reproduced as reported, actual gate effect unknown).
8. #350: candidate-label cache identity and overwrite behavior (helper-reproduced as reported), selection/context coverage (static/inferred), actual holdout isolation unknown.
9. #346: unsupported smoke and rollback confirmation gaps (static), #328 recurrence dedupe proposal; prove actual recovered service identity and behavior.

No finding above is classified as a demonstrated production exploit. Confirm against current code, tests and settings before creating fix PRs. Resolve each with regression proof, disproval, existing fix linkage or explicitly accepted residual risk. Do not mechanically implement all 21 documents.

### Documentation merge and retention

Review and merge topic documents individually under existing rules; leave #343 last, once references and shared index have been reconciled. Rebase/reconcile each shared docs/prior-art.md edit against actual main, preserving every accepted entry, link and source qualification. Do not overwrite previous index additions. Keep a single canonical design, plan and status. Historical records may be corrected with transparent dated disposition, not silently removed; normative changes need the repository's approval and ADR/design workflow. No merge, runtime change or implementation approval follows from this index.

**Checked:** connector PR inventory and original indexes, prior decision briefs as represented in this conversation, completed final Deep Research report. **Inferred:** consolidated ordering; validate exact scope with owner. **Not verified:** local make verify, CI, live enforcement, complete code re-audit or actual source text of every earlier cited study.
