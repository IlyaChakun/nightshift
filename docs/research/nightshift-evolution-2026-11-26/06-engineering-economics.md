# Engineering economics and adaptive execution

Optimize the cost of verified outcomes rather than the price of individual model calls. Use existing job and budget data to compare fixed execution policies before allowing any adaptive routing; safety gates and owner spending caps remain independent constraints.

**Priority:** P1. **Status:** research proposal, not approved implementation.

## Contents

- [Problem](#problem)
- [How it works](#how-it-works)
- [Trade-offs](#trade-offs)
- [Sources](#sources)
- [Fit](#fit)
- [Status](#status)
- [Proposal](#proposal)
- [Acceptance and stop conditions](#acceptance-and-stop-conditions)
- [Agent follow-up](#agent-follow-up)
- [Record and evidence](#record-and-evidence)
- [Decision brief — trustworthy economics before adaptive routing, 2026-11-26](#decision-brief--trustworthy-economics-before-adaptive-routing-2026-11-26)

## Problem

A cheap model can be expensive when it causes rework or consumes operator time. A subscription run with no marginal invoice still consumes scarce quota. Successful-only dashboards hide failed attempts, canceled work, and unresolved tasks. Static routing cannot show which policy works best for a given task class.

## How it works

Attribute all job attempts, verification, CI, infrastructure and operator handling to a task or feature. Keep cash spend, estimated shared subscription allocation, token consumption and human minutes separately visible. Report denominators, missing cost data, observation window and task mix.

Define cost per accepted outcome as total attributed cost of all attempts in a cohort divided by independently accepted outcomes in that cohort. Report the failure and cancellation counts alongside it; do not produce a zero denominator result. Include elapsed and active time separately, and measure escaped defects in a fixed follow-up window.

## Trade-offs

| Option | Benefit | Limitation |
| --- | --- | --- |
| Static role and provider routing | Predictable, easy to audit | May waste effort |
| Evidence-based rules by task class | Simple adaptation, interpretable | Sparse data and drift |
| Learned or bandit routing | Can explore alternatives | Exploration cost, bias and unsafe optimization |
| Single blended monetary objective | Simple comparison | Conceals quality and attention trade-offs |

Start with cohort reporting and static policy comparisons. Use a quality floor, spending cap and latency constraints before ranking cost. Never convert missing costs to zero.

## Sources

- [DORA software delivery metrics](https://dora.dev/guides/dora-metrics/), search excerpt checked 2026-11-26: use delivery throughput and instability together; do not equate PR volume with value.
- [OpenTelemetry GenAI inference conventions](https://github.com/open-telemetry/semantic-conventions-genai/blob/main/docs/gen-ai/client-inference.md), search excerpt checked 2026-11-26: reference for normalizing inference telemetry; verify stability and privacy before adopting.
- Uber case study in PR #339 is a prior research lead for routing and cost optimization.

## Fit

Design sections 7, 12 and 15 already define budgets, fallback and task timelines. ADRs 0012 and 0020 cover provider limits and model selection. The proposed work extends existing records rather than adding a billing or routing service.

## Status

Partial: plan S1–S3 documents supervisor, timeline and model/effort selection; live confirmation remains open in status. Comparative outcome economics and adaptive execution policy quality are not established by those features.

## Proposal

First validate accounting against real harness usage and reconcile invoices or provider totals where available. Build cohort reports from the book and an operator-time sample. Compare two fixed execution policies on paired frozen tasks under equivalent total constraints.

Capture model, harness, skills, policy version, task class, attempt outcome, total cost, human minutes, latency and acceptance evidence. Repeat stochastic tasks where budget permits; use paired differences and uncertainty ranges instead of ranking tiny samples.

Only after repeatable gains, propose a rule that recommends a policy by task class. Keep unknown classes on the approved default; route decisions remain logged, reproducible and reversible. Do not automatically change spending caps, provider eligibility, data classification or verification requirements.

## Acceptance and stop conditions

- Failed, canceled and escalated tasks contribute to cohort cost and outcome counts.
- Missing usage, subscription quota and cash cost are labeled separately.
- A cheaper policy that fails independent quality gates cannot win.
- Switching policy is bounded by existing owner-approved spend and provider constraints.
- A paired experiment demonstrates improvement in cost or human minutes at an agreed quality floor; otherwise retain the default.

## Agent follow-up

Coordinate with PRs #337, #342 and the experimentation proposal. Resolve live token accounting and decide the cost attribution convention before implementing routing changes.

## Record and evidence

Preserve this dated owner-requested proposal. Link later experiments, decisions and implementation PRs; do not silently delete or replace the source record. The living design remains `docs/design.md` and the canonical sources index is `docs/prior-art.md`.

**Checked:** [base 9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; design sections 7, 12, 15, 18, 20, plan, status, research index and open PR titles. **Inferred:** candidate gaps, priorities and experiment design. **Confidence:** medium; no live validation or full code audit. External links identified above were checked only to the extent stated.

## Decision brief — trustworthy economics before adaptive routing, 2026-11-26

**Decision:** treat measurement correctness as P1 and adaptive execution as a subsequent controlled experiment. Retain ADR 0020's strong Lead/Verifier defaults, existing router and owner-approved limits. Do not optimize for a provider's nominal token price, or build a second gateway or model router.

### Pinned implementation findings

| Evidence | Disposition |
| --- | --- |
| [Job schema](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/book/migrations/00003_jobs.sql#L6-L31), [job queries](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/book/queries/jobs.sql#L20-L68) | A job row has one usage/result record; reclaim increments attempt and reuses the row. Completion persists usage for the winning lease, but earlier interrupted attempts may have incurred usage. Job totals alone cannot establish complete physical-attempt economics. |
| [Usage/price](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/jobsapi/api.go#L71-L83) and [runner](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/runner/runner.go#L202-L218) | A reported zero cost triggers a token-price estimate; absent price also produces zero. The stored cost lacks an explicit source/cost category in the inspected usage struct. Distinguish reported, estimated, cash and unknown before aggregating. |
| [Claude meter](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/harness/claude/claude.go#L311-L395), [Codex parser](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/harness/codex/codex.go#L229-L250) | Distinct streaming counters; Claude's meter explicitly excludes cache tokens. Status records a fix for undercounting and a pending live re-check. Neither stream alone proves invoice accuracy. |
| [Supervisor](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/harness/supervise/supervise.go#L70-L100) and [end check](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/harness/supervise/supervise.go#L163-L194) | Best-effort spend enforcement from events; final backstop checks tokens, deliberately not cost. A job limit is not evidence of a hard aggregate provider billing cap. |
| [Router](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/router.go#L81-L168), [ADR 0012](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/adr/0012-limits-wait-then-fallback.md), [ADR 0020](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/adr/0020-model-and-effort-per-role.md) | Existing wait/fallback, concurrency and role/model/effort configuration are the right baseline. Verify enforcement of every monetary/share cap in actual call paths; no complete cap audit was performed. Do not equate ADR promises with implemented enforcement. |
| [Task timeline SQL](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/book/migrations/00016_dash_task_timeline.sql#L14-L29), [summary queries](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/book/queries/summary.sql#L4-L33) | Useful operational views, but zero cost is hidden, durations span reclaimed attempts, and successful/merged task counts are not independently accepted objective cohorts. Keep these views; add economics as a separate read model from the same book. |

### External evidence and limits

Checked 2026-11-26. These references motivate hypotheses, not projected Nightshift savings.

- [FinOps Foundation, Unit Economics](https://www.finops.org/framework/capabilities/unit-economics/): distinguish resource-efficiency units from business outcomes; establish allocation rules, baselines and affordability constraints. Use this maturity sequence: reliable direct accounting first, fully loaded outcomes when trustworthy.
- [DORA metrics](https://dora.dev/guides/dora-metrics/): measure delivery throughput and instability together, at the application/service level. Do not optimize PR count or use deployment frequency as a productivity target.
- [OpenTelemetry GenAI client inference](https://github.com/open-telemetry/semantic-conventions-genai/blob/main/docs/gen-ai/client-inference.md): requested/actual model, inference duration and input/output/cache/reasoning token dimensions. The current document marks itself Development; the former OTel metrics page redirects readers to this repository. Pin the convention revision before adopting it. Use OTel for observability, not the sole authoritative billing ledger; do not emit prompts or high-cardinality task IDs as metric labels.
- [FrugalGPT, 2023 preprint](https://arxiv.org/abs/2305.05176) and [RouteLLM, ICLR 2025 abstract](https://proceedings.iclr.cc/paper_files/paper/2025/hash/5503a7c69d48a2f86fc00b3dc09de686-Abstract-Conference.html): model cascades and learned routing can improve cost/response-quality trade-offs on the authors' benchmarks. The latter reports more than twofold cost reduction in its evaluation. Neither establishes the economics of long-running coding jobs, rework, independent acceptance, operator involvement or Nightshift's subscriptions. Do not import their router as a dependency.
- PR #339's Uber case and #342's METR/SPACE references give complementary reasons to count operator time and hold the model constant when evaluating workflow changes. Do not double count the same studies as independent evidence.

### Accounting contract and aggregation

Start with one protected pilot ledger or read model using existing book IDs. The following are proposed accounting dimensions, not existing schema fields.

| Quantity | Rule |
| --- | --- |
| Physical attempt | Unique job ID + lease attempt, tenant, role, provider, requested/observed model, policy version and timestamps. Record start, terminal or interrupted state, usage completeness and provenance. A failed/canceled/reclaimed attempt can spend money. |
| Cash spend | Actual attributable provider/API/credit charges, infrastructure and evaluation expenses; currency and billing-period provenance. Reconcile with available provider totals, retaining unattributed residuals. |
| Estimated model cost | Versioned rate card and observed token classes, or provider-reported notional value; report the estimate separately from cash. Cache/ reasoning categories must use the provider's inclusion rules to avoid double counting. |
| Subscription | Fixed invoice, attributable share using a declared allocation rule (e.g. measured eligible consumption/time), and observed limit/reset events. The marginal cash charge may be zero while quota opportunity cost is nonzero. If consumption is unavailable, mark allocation uncertain, not free. Never add notional API pricing to the same subscription invoice as cash. |
| Operator time | Active specification, approval, review, rescue, operations, and allocated maintenance minutes (#348); keep shared time and overlapping intervals deduplicated. Optional owner-selected hourly value is a scenario, not a cash invoice. |
| Cohort and outcome | Freeze objective IDs/eligibility and arm before execution; include all starts, failed, abandoned, canceled, blocked and unresolved with their costs. Distinguish independently accepted, released, handed off and merged. |

For a cohort with N independently accepted objectives, use total attributable cost across all assigned work divided by N. Calculate cash and estimated fully loaded versions separately, with scope and coverage. If N is zero, ratio is undefined. If usage is missing, show known spend, coverage and any justified bound; never claim a complete total. Report raw counts, paired outcomes and uncertainty. Infrastructure allocation and subscription attribution must sum to the included shared amount or expose the residual. Reconcile multiple telemetry streams for a physical attempt without double-counting both cumulative snapshots and deltas.

Illustration only: on ten comparable assigned objectives, policy A spends USD 20 but accepts four (USD 5/accepted); B spends USD 40 but accepts nine (USD 4.44/accepted). A's lower per-start price is misleading. This example is not a measured result; quality, human time and safety can still overturn the monetary ranking.

### Minimal implementation and validation sequence

1. Re-run the pinned harnesses on approved synthetic workloads: success, model limit, supervisor stop, error, cancellation, and reclaimed lease. Compare persisted usage, raw provider reports and billing/quota records where accessible. Check streaming cumulative/delta semantics and model substitution. Preserve missing-cost and incomplete-usage flags; record whether the amount is cash, reported estimate or locally estimated.
2. Track physical attempts separately from logical result consumption (A03). Account for usage even when the attempt is rejected as stale; it cannot modify authoritative task results. Use idempotent usage ingestion and bounded checkpoints if existing events are insufficient. No paid calls or new credential without approval.
3. Verify enforcement, not only reporting, of provider eligibility, configured quotas and aggregate monetary caps. Test concurrent starts near a cap, incomplete/unknown pricing, restart, late usage, fallback and unreported final cost. If a hard spending limit is promised, use a trusted pre-dispatch reservation or externally enforced provider cap; reconcile actuals, release unused reservation and block uncertain overspend. A streaming stop signal cannot retroactively prevent a provider charge. Treat this as a proposed fix subject to confirmed policy gaps, not proof of a currently exploitable cap.
4. Build the initial read-only cohort report and reconciliation from the book. Keep cost provenance and unknowns visible; use existing timeline/digest/Grafana for presentation. Do not add a billing service before local accounting proves insufficient.
5. Compare fixed policies on the paired frozen tasks defined in #342, varying one factor at a time (Builder model/effort first; retain fixed independent acceptance and Lead/Verifier settings). Equal total constraints, identical base/spec, blinded grading, and all attempts included. Budget calibration and declare meaningful effect and quality floor before main evaluation.

### Adaptive execution decision

Start with a deterministic policy table recommending an already-approved model/effort for an independently defined task class. Separate quality-driven escalation after a failed check from availability-driven fallback in ADR 0012. Unknown, critical or insufficient-evidence classes keep the approved strong default. The router still enforces provider/data eligibility, spending limits, concurrency and safety policy. Log why a policy was selected, its version and alternatives; maintain a rollback to the baseline. Never allow the agent to weaken verification, approve spending or change the price table.

Consider a learned router only after multiple task classes show stable, repeatable policy differences and its expected marginal savings justify maintenance, additional inference, exploration and drift monitoring. Shadow recommendations first; no uncontrolled exploration on live products. Apply #350's independent promotion criteria. Use a conservative stopping rule: zero tolerated authorization violations, no required acceptance regression, meaningful improvement in cost or operator time with uncertainty reported, and no unapproved spending. If inconclusive, retain current config; if overhead dominates, simplify the workflow.

**Checked:** pinned migrations, job API, runner, supervisor, router, harness usage parsing, timeline/summary and ADRs; primary documentation and paper abstracts above. **Inferred:** cost completeness risk, proposed accounting contract and routing strategy. **Confidence:** high for inspected representation, medium for actual usage/cap behavior and expected economics. **Not verified:** full caller/cap inventory, actual billing or subscription consumption, live harness accounting, local tests, statistically powered pilot or model-routing gains. A broader connector inventory hit its call limit; no negative assertion about uninspected paths is made.
