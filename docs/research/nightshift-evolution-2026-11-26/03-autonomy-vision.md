# Nightshift — autonomy maturity vision

**Status:** proposed strategic roadmap and maturity model; not an industry standard or current capability certification. **Date:** 2026-11-26.

## Contents

- [North star](#north-star)
- [Baseline and source grounding](#baseline-and-source-grounding)
- [Level 1 — Autonomous Task Execution](#level-1--autonomous-task-execution)
- [Level 2 — Autonomous Software Delivery](#level-2--autonomous-software-delivery)
- [Level 3 — Autonomous Product Development](#level-3--autonomous-product-development)
- [Level 4 — Self-Improving Factory](#level-4--self-improving-factory)
- [Level 5 — Self-Evolving Software Organization](#level-5--self-evolving-software-organization)
- [Two distinct feedback loops](#two-distinct-feedback-loops)
- [Recommended sequence and non-goals](#recommended-sequence-and-non-goals)
- [Maintaining the roadmap](#maintaining-the-roadmap)
- [Deep research validation — evidence-governed autonomy](#deep-research-validation--evidence-governed-autonomy)

## North star

Nightshift is an autonomous software engineering platform that turns authorized product intent into verified software outcomes, operates those outcomes, learns from evidence, and improves its own engineering process under a trusted control boundary. Human attention shifts from writing code and managing individual jobs to choosing goals, approving high-impact decisions and resolving exceptional uncertainty. Autonomy is not unlimited authority, nor does it imply an absence of accountable humans.

The factory should remain model-, harness- and framework-agnostic where meaningful. Preserve the Go/PostgreSQL/River foundation unless measured operational evidence warrants changing it. Increasing autonomy requires stronger verification and controls, not looser controls.

## Baseline and source grounding

The previous external audit of Nightshift described Go-based deterministic orchestration, PostgreSQL/River, event history/outbox, dispatcher/supervisor, Lead/Builder/Verifier, harness adapters, policy gates, tenant-oriented separation, artifacts and release workflows. Its reading of `docs/status.md` reported early phases implemented, 22/22 local recovery drills, one flow to `released`, live Builder runs and several full-flow, deployment or isolation checks not yet proven in real environments. These were reported results, not independently reproduced benchmarks. The precise commit is unknown; a fresh GitHub fetch failed when preparing this roadmap. No statement below certifies HEAD.

Repository navigation: [design](https://github.com/IlyaChakun/nightshift/blob/main/docs/design.md), [status](https://github.com/IlyaChakun/nightshift/blob/main/docs/status.md), [prior art](https://github.com/IlyaChakun/nightshift/blob/main/docs/prior-art.md), [durability research](https://github.com/IlyaChakun/nightshift/blob/main/docs/research/durable-execution.md), [agent rules](https://github.com/IlyaChakun/nightshift/blob/main/AGENTS.md), [dispatch verifier](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/verify.go). Reconcile with the latest ADRs, code, tests and audit package (NS-A01 through A07). These are mutable navigation links, not pinned evidence.

## Level 1 — Autonomous Task Execution

**Capability:** an agent independently handles a well-defined engineering task in a bounded environment, produces a patch and evidence, and stops or escalates when blocked.

**Existing foundation (historical audit):** Builder and harness adapters; job execution, artifacts, budgets and test infrastructure.

**Required proof:** actual task runs using real harnesses, isolated credentials/workspace, bounded iterations, reproducible artifacts and independently evaluated outcomes. Failed/cancelled/abandoned runs remain visible. A single successful run is not proof of reliable autonomy.

**Exit gate:** a repeated representative task cohort meets owner-defined quality and cost targets; workspace isolation, timeouts, cancellation and cleanup are verified. Report unplanned intervention separately from initial task specification.

## Level 2 — Autonomous Software Delivery

**Capability:** Nightshift owns the lifecycle from approved spec to independently verified release, with required approvals and recovery across agent/worker failures.

**Existing foundation:** the audited dispatcher, state machine, gates, River jobs, outbox, merge/release pipeline, supervisor and Verifier. Historical maturity assessment: Nightshift appeared to be implementing this level; operational evidence was insufficient to certify completion.

**Work:** resolve fail-open gates and durable domain decisions; atomic result consumption and idempotent side effects; scoped sandbox isolation; release approvals bound to the verified commit/spec/policy; real preview, acceptance, release and rollback. Use audit NS-A01/A02/A03/A04/A07. Adopt Stripe's isolated workflow and OpenAI's legible runtime techniques selectively.

**Exit gate:** repeat real E2E delivery with valid evidence; injected worker crash and duplicate deliveries recover without lost tasks or unauthorized side effects; invalid/stale approvals and missing checks block release. Show observed outcomes, recovery rates and total cost. Expand production access only after threat-model-specific gates pass.

## Level 3 — Autonomous Product Development

**Capability:** decompose an authorized product objective into dependent work and verify the integrated product, rather than merely completing individual PRs.

**Gap hypothesis:** earlier audit found task-level verification and apparent omission of parent criteria from some child verifier context. Verify whether current code already handles feature acceptance, dependencies and invalidation.

**Work:** versioned parent objectives, minimal dependency tracking, requirements-to-evidence traceability, integrated acceptance, cross-task contract/regression tests, bounded replanning, product-profile context, tenant-specific objectives and outcomes. Reuse Lead/Builder/Verifier initially. Use [tenant product intelligence](02-tenant-product-intelligence.md) in advisory mode to surface missing evidence. See NS-A05/A06.

**Exit gate:** one multi-task product feature passes independent integrated acceptance; deliberately incompatible children cannot mark the objective complete; changed requirements invalidate affected evidence. Deliver and operate a small pilot product with real feedback and a documented baseline. Feature completion is not proof of customer value; validate separately.

## Level 4 — Self-Improving Factory

**Capability:** turn observed failures and opportunities into tested improvements to the factory's tools, policies, harness, workflows and knowledge. Promote only when independently validated. Self-improvement may improve success rate, human effort, cost or recovery, not merely expand autonomy.

**Existing foundation:** earlier audit identified self-improvement plans, knowledge/memory, governance, telemetry and a preference for mechanical enforcement over accumulating instructions. Actual controlled self-upgrade effectiveness remained unproven.

```
Build / operate products with trusted factory N
    -> collect scoped outcomes, failures and interventions
    -> classify cause and select improvement hypothesis
    -> propose candidate N+1 (isolated branch/environment)
    -> regression tests + representative evals + hidden holdouts
    -> independent policy and release authority checks
    -> canary on limited eligible work
    -> compare quality, cost, attention and safety
    -> promote N+1 OR reject/rollback to N
    -> continue measuring with retained evidence
```

**What to learn:** environment gaps, recurring verifier findings, failed recovery, repeated unnecessary tool calls, stale knowledge, model routing, missing deterministic checks and poor specifications. Do not automatically treat model-written explanations as correct root causes. Improvements may be a skill, tool, schema, regression test, workflow change or deliberate removal of unnecessary complexity. Track candidate lineage, versions, hypothesis, experiment results and rollback.

**Trust boundary:** independent authority owns promotion, protected policy, holdout inputs and safety invariants. The candidate must not redefine its own pass criteria or gain privileges. Freeze baseline and metrics before evaluation; do not let hidden holdouts leak into training/repair context. Sanitize tenant data and obtain explicit permission for reusable cross-tenant learning; prefer generalized non-sensitive patterns. A sandbox is not proof of safety by itself. See NS-A02/A04/A06/A07.

**Exit gate:** demonstrate at least one repeatable improvement on a representative baseline plus independent holdout, no safety regression, measured resource/quality trade-off, limited canary and verified rollback. Show the factory can reject a harmful candidate. Require sufficient repetitions before asserting a sustained gain.

## Level 5 — Self-Evolving Software Organization

**Capability:** operate a portfolio of authorized products, observe user/operational signals, propose or execute bounded changes, and allocate attention and budgets according to owner-defined strategy. Product learning and factory learning interact only through explicitly permitted interfaces.

**Work:** tenant/product-level profiles and authority policies; evidence ingestion; opportunity triage; hypothesis and experiment tracking; outcome monitoring; production incident workflows; portfolio-level constraints, conflict resolution and owner decisions. Learn from the OpenAI Performance Factory's alert-to-recovery model and Ona's lifecycle experiment. Implement advisory mode first, then progressively authorize low-risk actions with audit and stop controls.

**Exit gate:** over multiple observation windows, show that recommendations are grounded in real user evidence, reversible experiments are authorized and evaluated, results inform future decisions, and cost/security/tenant isolation remain within limits. Demonstrate a decision to stop or not build an idea, not just feature throughput. Commercial outcomes require customer validation; do not infer product-market fit from test success.

## Two distinct feedback loops

| Product loop | Factory loop |
|---|---|
| What problem should this product solve? | How can Nightshift deliver better? |
| Tenant-scoped strategy and user evidence | Scoped engineering failures, reusable patterns, evaluations |
| Opportunity → experiment → delivery → outcome | Failure → candidate improvement → independent evaluation → canary/promotion |
| Owner-approved business policy | Independent platform promotion authority |
| Product success/guardrails | Engineering quality/cost/reliability/attention |

A product may use a mature delivery engine while keeping product autonomy off. A factory may improve its test runner without gaining any production or tenant permissions. Track maturity separately by capability and tenant/product policy; these five levels are a roadmap, not a global permission flag.

## Recommended sequence and non-goals

1. Stabilize level 2 safety and recovery; establish live baseline and launch the pilot (see [04-product-pilot-and-measurement.md](04-product-pilot-and-measurement.md)).
2. Add level 3 integrated acceptance and minimal product context in response to actual pilot friction. Track requirements, quality and outcomes.
3. In parallel, capture learning candidates, but keep promotion manual/independently controlled until level 4 evaluation is proven.
4. Introduce a narrow product-discovery advisory workflow and production feedback workflow, then bounded reversible actions only after evidence supports them.
5. Revisit framework changes or extra agent roles only when measured bottlenecks justify them.

Do not build a generic autonomous organization, an analytics warehouse, or dozens of specialist agents in advance of actual product usage. Do not treat LOC, PRs, agent count, successful fake tests or model confidence as maturity gates. Avoid self-modification of protected controls and irreversible external actions without explicit authority.

## Maintaining the roadmap

For each level maintain a matrix of current HEAD, evidence (run/commit), verified capability, remaining gaps, owner, proposed experiment, decision and next gate. Link existing issues/ADRs, do not duplicate backlog. Reassess after each pilot cohort and factory candidate; publish failures as well as successes. Maturity should be demonstrated by actual behavior under representative conditions, including negative cases.

## Repository reconciliation and retention

This dated external research proposal is not a normative design change. Preserve the snapshot and append links to subsequent decisions and evaluation evidence rather than deleting it when superseded. Current status documents phases 0–4 built, phase 5 on fakes and phase 6 on stubs. Level 2 is a target with partial evidence, not an achieved certification. Keep `docs/design.md` the only living design; owner approval and ADR/design sync precede any adopted architectural changes.

**Checked:** repository rules, status and research index at [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26. External source dates and links are listed in the research pack. **Inferred:** proposed architecture, experiments and maturity targets; no implementation or runtime validation. **Confidence:** medium.

## Deep research validation — evidence-governed autonomy

**Date:** 2026-11-26. Transferred from the completed Deep Research report "Nightshift deep research: final block", with connector reconciliation. The report could not pin main through its web interface; actual connector read returned [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15). Earlier unpinned observations remain historical, not assertions of current capability. No new runtime tests or measured autonomy gains.

Preserve the five-level maturity model as a proposed roadmap, not a certification or permission switch. Every higher level must expand scope of responsibility while raising the independent evidence floor.

| Level | Required demonstration beyond the prior level |
| --- | --- |
| 1 | Representative bounded task outcomes with independently evaluated artifacts and observed resource/intervention cost |
| 2 | Exact-candidate acceptance, authorized release, durable recovery and negative safety cases on a real environment |
| 3 | Integrated product objective passes independently; cross-task failure cannot be hidden by local task success; actual user outcome remains distinct |
| 4 | Candidate improves a frozen baseline under protected evaluation; it cannot authorize its own promotion; rollback demonstrated |
| 5 | Repeated owner-authorized product decisions grounded in actual evidence, including no-build/stop decisions, measured outcome and guardrails |

Level 5 may be described as an **Evidence-Governed Software Organization**; this clarifies the existing "Self-Evolving Software Organization" label without deleting the historical model. Strategic direction, creation of trust domains, spending expansion, protection weakening and irreversible actions outside approved policy remain human authority. Do not conflate more roles, more retries, or more code with autonomy.

### External calibration

Report primary-source excerpts checked 2026-11-26; no full benchmark reproduction:

- [Anthropic, Building effective agents, 2024-12-19](https://www.anthropic.com/research/building-effective-agents): increase autonomy only where task structure warrants it; environmental feedback, stopping conditions and sandbox evaluation matter.
- [Open SWE](https://github.com/langchain-ai/open-swe): a coding workflow using isolated environments, curated tools and review feedback. Its rapidly evolving implementation is not proof of a fully autonomous organization; evaluate actual credential scopes.
- [OpenAI Symphony specification](https://github.com/openai/symphony/blob/main/SPEC.md): issue-driven workspace/run scheduling, not a general multi-tenant control plane. Reuse isolation and bounded-concurrency principles.
- [StrongDM Attractor specification](https://github.com/strongdm/attractor/blob/main/attractor-spec.md): explicit graph traversal, retries, goal gates and human waits. Specification, not independent production outcome data; no reason to replace existing Go/Postgres orchestration solely on this basis.
- [METR time horizons](https://metr.org/time-horizons/): calibrated task difficulty by human completion time at specified success probability on a particular suite; not duration of uninterrupted autonomous agent operation. Do not turn the benchmark into a Nightshift maturity KPI.

### Evidence ledger and sequencing

Report maturity per capability, tenant policy and environment, not a single global number. Record independently accepted objective count, all starts and failures, unplanned human rescue, approved mandatory decisions, resource costs, recovery and safety events. Use #342/#345/#348 for the denominator and actual intervention burden. Mandatory approvals are not undesirable interruptions to eliminate. Unknown or missing required evidence blocks level claims. Owner defines thresholds before testing; small cohorts support a pilot decision, not certification.

A01/A02/A03 establish shared authority and durability; A04/A07 prove isolation and live operation. A05/A06, #347 and #348 establish objective completion, convergent repair, current-candidate and decision integrity. #350 provides protected experiments for level 4; #351 sharing remains opt-in, tenant-local first. #340/#346 provide distinct product and reliability loops. #352 is contingent on measured needs. Reconcile overlaps in #343, not another execution platform.

**Decision:** keep the vision; prioritize proof of level 2 and a real pilot before expanding scope. Reject unbounded self-authorization. **Confidence:** high for sequencing, medium for future capability; **not verified:** actual attainment of any higher level, live evaluation, customer value or organizational economics.
