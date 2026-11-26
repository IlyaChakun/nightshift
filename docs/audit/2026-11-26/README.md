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
