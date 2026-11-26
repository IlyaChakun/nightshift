# Engineering economics and adaptive execution

Optimize the cost of verified outcomes rather than the price of individual model calls. Use existing job and budget data to compare fixed execution policies before allowing any adaptive routing; safety gates and owner spending caps remain independent constraints.

**Priority:** P1. **Status:** research proposal, not approved implementation.

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
