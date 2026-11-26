# NS-A05 — Product-level orchestration and completion

**Priority:** P2; elevate if current production flow marks parent objectives complete without required feature acceptance.  
**Status:** architectural gap hypothesis, not a verified missing feature.

## Observation

The earlier review characterized Nightshift as task-oriented (idea/spec/task/build/verify/PR/release). In [verify.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/verify.go), `verifierContext()` appeared to omit the parent specification for decomposed tasks because its acceptance criteria apply to the overall group. This is reasonable for child-level verification if there is an independent parent acceptance stage. Inspect existing parent/child, merge-train, dependency, specification and release mechanisms to determine whether that stage already exists.

## Why it matters

Passing every local task does not prove the product objective. Interfaces can mismatch, two individually correct changes can interact incorrectly, or a requirement can be lost during decomposition. A feature needs evidence over its integrated state, not merely a count of completed children.

## Proposed invariants

- A parent objective has versioned acceptance criteria and explicit completion state.
- Every required criterion maps to evidence at an appropriate task, integration or feature level.
- Parent acceptance evaluates the integrated artifact containing the relevant completed tasks, under the current parent revision.
- Changes to upstream requirements or contracts invalidate affected evidence; failed or blocked dependencies cannot be silently treated as completed.

## Options and trade-offs

1. Minimal parent acceptance gate: attach parent spec revision and an integration scenario suite to the existing aggregate task/feature record. Low complexity; may be enough for sequential workflows.
2. Explicit dependency DAG with readiness rules, blocked states and traceability. Enables parallel execution and dependency-aware replanning; requires cycle detection, concurrency rules and invalidation semantics.
3. Dynamic planner that revises the DAG using agent judgment. Handles discovery and changing requirements; adds non-determinism, requires versioned plans and bounded replanning, and must not weaken original acceptance criteria without authorization.

Prefer option 1 if existing decomposition is sufficient; introduce a DAG only for demonstrated dependency complexity. Keep scheduling and acceptance deterministic; delegate requirement interpretation and proposed plan changes to agents. Reuse existing Lead/Builder/Verifier roles rather than adding roles by default.

## Candidate techniques

Requirements traceability (criterion IDs to tests/evidence), contract tests, semantic merge conflict detection, integrated feature previews, regression testing and controlled plan revisions. These are alternatives/tools, not mandatory new subsystems. Semantic conflicts are behavior-level conflicts even when Git merge succeeds; prioritize integrated tests over speculative semantic-analysis infrastructure.

## Acceptance criteria

- A two-task feature with incompatible interfaces passes child tests but fails parent acceptance; no completed parent status is emitted.
- A missing requirement or failed dependency prevents feature acceptance.
- A corrected integrated artifact passes and is linked to the current parent spec revision and artifact digest.
- Changing the parent spec or upstream contract invalidates relevant evidence and schedules appropriate checks without unnecessary full reruns.
- Parallel work, partial failure and cancellation have explicit semantics; dependency cycles are rejected if a DAG is used.
- Demonstrate an end-to-end multi-task feature through the real workflow, not only unit-level helpers.

**Dependencies:** NS-A01 evidence provenance and release integrity; NS-A03 durable transitions.

## Follow-up source check and retention

This dated external review is a candidate finding record, not the living design. Preserve the original findings and append a disposition or link to implementation evidence; do not delete this record merely because an issue is resolved. Superseding or relocating it requires an explicit owner decision.

**Checked:** pinned Nightshift base [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; `docs/status.md` and `docs/research/README.md`. Current status records phase 5 Lead and hidden scenarios on fakes; product-level completion and cross-task acceptance require fresh code validation.

**Inferred:** priorities and proposed solutions; no tests or live runs performed. **Confidence:** medium.
