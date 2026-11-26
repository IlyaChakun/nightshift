# NS-A05 — Product-level orchestration and completion

**Priority:** P2; elevate if current production flow marks parent objectives complete without required feature acceptance.  
**Status:** architectural gap hypothesis, not a verified missing feature.

## Contents

- [Observation](#observation)
- [Why it matters](#why-it-matters)
- [Proposed invariants](#proposed-invariants)
- [Options and trade-offs](#options-and-trade-offs)
- [Candidate techniques](#candidate-techniques)
- [Acceptance criteria](#acceptance-criteria)
- [Follow-up source check and retention](#follow-up-source-check-and-retention)
- [Decision brief — integrated objective acceptance, 2026-11-26](#decision-brief--integrated-objective-acceptance-2026-11-26)

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

## Decision brief — integrated objective acceptance, 2026-11-26

**Disposition:** the task DAG already exists. Confirmed static gap in the inspected decomposition/verification path: the original task is reused as the first implementation task and its whole-spec context is removed from review after expansion; sibling creation does not persist explicit parent membership or per-child criterion allocation. No aggregate acceptance gate was found in those inspected paths. Elevate to P1 before claiming multi-task product delivery; keep P2 for optional portfolio-level planning. No live false-completion incident was reproduced.

### Pinned code evidence

| Evidence | What it establishes |
| --- | --- |
| [Spec schema](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/schemas/spec.json#L14-L25) and [task entries](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/schemas/spec.json#L39-L49) | The approved spec already has AC IDs, Given/When/Then and optional source quotes; task entries have ID/title/repo/depends_on, not criterion mappings. Do not create another spec format before reusing these. |
| [Graph validation](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/deps.go#L39-L91) | Explicit DAG, duplicate/unknown/self-edge/cycle checks already built. Original option 2 is a foundation, not new scope. |
| [Expansion](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/deps.go#L94-L178), [CreateSpecTask SQL](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/book/queries/tasks.sql#L53-L76) | One transaction locks the source task, renames it to first task title, creates siblings sharing chat thread, links DAG, emits spec.expanded with empty object. The ID map is local to expansion. Shared thread is a presentation link, not a stable aggregate membership/version record. Siblings with no dependency edge need explicit membership too. |
| [Verifier context](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/verify.go#L157-L183) | Explicitly drops the full spec after spec.expanded, because criteria belong to the whole set. Correct intent for task-local review, but no criterion subset is substituted here. |
| [Builder enqueue](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/build.go#L230-L280), [specOf](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/lead.go#L517-L529) | Context is task-scoped; first task retains original spec event, siblings receive titles and shared thread, not the original event by this creation path. Verify effective prompts on a two-task fixture. |
| [Hidden scenarios](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/hidden.go#L16-L60) | Stored by tenant + task ID, superseded when new spec arrives; not an aggregate versioned acceptance run. A01 separately confirms the need to wire mandatory acceptance rather than assume it occurs. |
| [Dependency closure](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/deps.go#L196-L276), [release train](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/train/train.go#L247-L272) | Dependency readiness uses released/merged-by-team; owner may proceed after removing broken dependency links. Each task can reach its own release outcome. A merged external task or waived scheduling dependency is not automatically evidence that a parent requirement is satisfied. |

### Research basis and trade-offs

Primary search excerpts checked 2026-11-26:

- [NASA Systems Engineering Handbook, Appendix D](https://www.nasa.gov/reference/appendix-d-requirements-verification-matrix/) identifies each requirement by unique ID and source. [NASA SWE-052 bidirectional traceability](https://swehb.nasa.gov/spaces/SWEHBVD/pages/102695427/SWE-052%2B-%2BBidirectional%2BTraceability) connects requirements to test procedures/evidence. Transfer the lightweight traceability principle, not aerospace documentation bureaucracy.
- [Anthropic, Building effective agents, 2024-12-19](https://www.anthropic.com/research/building-effective-agents): orchestrator-workers decomposes dynamically; evaluator-optimizer fits clear criteria and useful refinement. Neither guarantees that worker success implies objective success. Use deterministic lifecycle/evidence and model-proposed plans.
- #339 StrongDM and #347 Pact/integration-candidate research already address independent scenarios and combined compatibility. Reuse those mechanisms rather than create duplicate evaluators or schedulers.

Compare (A) convention-only final integration task: cheap pilot, but a missing task or canceled dependency can evade closure unless code enforces aggregate completion; (B) a small durable objective/plan record with one aggregate gate over the existing DAG: recommended when implementing product-level completion; (C) a general dynamic hierarchical workflow engine: premature; require real evidence that B cannot support a tenant's objectives. Preserve task lifecycle, train, and existing spec/approval authority.

### Minimal objective semantics (proposal)

1. At approval, preserve original goal, spec revision/digest, required AC IDs and hidden suite version separately from implementation-task identity. Persist objective membership and the spec-local T-ID → book task ID mapping in the same expansion transaction; make retries idempotent. A one-task objective uses the same logical contract without duplicating execution. Decide schema/ADR with owner; not a new canonical model in this audit.
2. Allocate each required criterion to one or more task checks and/or an integration check. Require complete traceability; a criterion may span children. Validate coverage, unique IDs and ownership before dispatch. Keep hidden scenarios inaccessible to Builder; provide only public behavior criteria and redacted failure evidence.
3. Keep task outcomes distinct from objective outcomes. Proposed objective milestones: approved, implementing, integration pending, accepted, released/observing, blocked/canceled. Completion requires every required criterion to have valid passing evidence against the exact integrated revision(s), policy and spec. Required NOT_RUN/unknown is not pass. Partial release and partial progress are explicit. A human can authorize a revised requirement, not silently convert a failed requirement into satisfied.
4. Produce an integrated candidate containing required changes. In a single repository use a known combined tree or staged release, verify actual included changes and current-base evidence (#347). For multi-repo work pin a manifest of versions and compatible deployment order; separate merges are not atomic. For incremental release, either prove each intermediate state is safe or keep incomplete behavior behind a feature flag. Do not make a final gate block already-authorized safe infrastructure increments, but never mark the entire feature delivered prematurely.
5. Run an independent objective-level Verifier using the full current approved spec and integrated environment, including required hidden scenarios and cross-task behavior. Use A01 release evidence and A03 durable transitions; keep separate acceptance and production activation authorities. A failed aggregate check routes bounded corrective tasks with explicit linkage; do not erase the original intent or grant a new scope. If the spec genuinely changes, owner approval and a new revision invalidate affected evidence; never silently edit earlier criteria or membership.
6. Record user outcome after release separately (#342/#340). Technical acceptance demonstrates that specified behavior works; it does not demonstrate demand, adoption or value.

### Acceptance matrix

| Case | Required result |
| --- | --- |
| Two child tasks pass local checks, shared API mismatch | Integrated objective fails; no accepted/completed aggregate |
| Independent sibling without dependency link | Included in explicit membership; its missing evidence blocks required criteria |
| Expansion crashes/retries, two concurrent expansions | Same objective revision and membership, no duplicate siblings |
| First child released early, later child canceled | Partial objective state; no completed parent. Waiving dependency only permits scheduling, not criterion waiver |
| Changed spec, changed candidate or external merge | Stale evidence rejected; re-evaluate affected scope conservatively |
| Required hidden suite absent/unavailable | NOT_RUN/blocked; no pass; Builder cannot read hidden suite |
| Correct multi-task feature | Full independently verified criterion coverage; exact integrated artifact; approved release and user outcome remain separately recorded |
| Failed aggregate check repeats | Bounded replan/repair or owner escalation (#337); no unbounded DAG expansion |

Run an inexpensive pilot first: one explicit integration task with two independent changes on a synthetic application, frozen parent spec and external test. Exercise the current implementation and record actual prompts, state events and missing acceptance evidence. Then implement the minimal aggregate model only for confirmed gaps, with Postgres/restart tests and a real staging objective. Measure missed criteria, unnecessary repeated tests, accepted objectives, human attention and cost. No runtime implementation or production rollout is approved by this record.

**Checked:** pinned schema, expansion, context, dependency, release and hidden storage paths; NASA and Anthropic primary search excerpts. **Inferred:** objective record, scope and prioritization. **Confidence:** high for the static omissions, medium for complete repository coverage. **Not verified:** entire code inventory, actual multi-task run, runtime bypass, local tests and CI.
