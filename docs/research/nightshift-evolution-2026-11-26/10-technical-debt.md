# Technical debt and architecture evolution

Turn recurring maintenance findings into prioritized, justified architecture work. Track the cost of debt and enforce only proven architectural invariants; avoid a permanent autonomous refactoring treadmill.

**Priority:** P2. **Status:** research proposal, not approved implementation.

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
- [Decision brief — debt as observed maintenance cost, 2026-11-26](#decision-brief--debt-as-observed-maintenance-cost-2026-11-26)

## Problem

Fast autonomous delivery can increase duplication, dependencies, coupling and accidental divergence from architecture decisions. A quality dashboard does not explain which debt matters. Agents can spend large budgets fixing cosmetic findings while delaying useful product work.

## How it works

Reuse findings and the book as the source for a debt view. A proposed debt record links evidence, affected component, known invariant, observed consequence, estimated remediation and ongoing cost, confidence, owner and suggested review trigger. Distinguish security defects, reliability risks, architectural drift and optional cleanup.

An architecture fitness function is a proposed executable check of an accepted architectural property: for example a prohibited dependency direction, a contract compatibility requirement or a migration safety invariant. Derive each rule from an accepted ADR or observed failure; establish a baseline, test planted violations and use the existing trial-to-active lifecycle.

## Trade-offs

| Option | Benefit | Limitation |
| --- | --- | --- |
| Weekly audit and manual prioritization | Minimal change | Recurring findings may be forgotten |
| Debt view and targeted fitness checks | Evidence-driven prioritization | Requires maintained ownership and baselines |
| Automatic opportunistic refactoring | Can remove localized friction | Scope creep, behavior changes and cost |
| Global complexity score or hard quota | Easy to graph | Goodhart effects and arbitrary incentives |

Prefer a small debt view derived from existing findings. Keep existing mandatory checks. Add debt limits per accepted policy and stack only where evidence shows harm; use trends and contextual review rather than universal LOC or complexity targets.

## Sources

- [NIST SSDF 1.1](https://csrc.nist.gov/pubs/sp/800/218/final), search excerpt checked 2026-11-26: secure development and vulnerability mitigation reference; this proposal does not assert compliance.
- [DORA delivery metrics](https://dora.dev/guides/dora-metrics/), search excerpt checked 2026-11-26: rework and delivery instability help evaluate whether debt remediation matters.
- Design sections 5, 9 and 10 contain the canonical standards, audit and rule lifecycle.

## Fit

The design already defines complexity and duplication checks, new-violation baselines, weekly audit, findings clustering and rule promotion. Reuse those mechanisms rather than introduce another quality engine. Architecture decisions remain in ADRs, and rules that change policy require the established review.

## Status

Partial: audit and standards foundations are documented, and the plan includes unfinished quality checks. Debt cost attribution, explicit deferral decisions and longitudinal evidence are proposals; code-level coverage is unverified.

## Proposal

Choose one recurring failure from a pilot or the Nightshift tenant. Record its history, measured rework and affected invariant. Compare doing nothing, a small refactor and a stronger automated check. Select the cheapest intervention that prevents the actual problem; evaluate false positives and regression risk before activation.

For optional cleanup, require a scoped task with behavior-preserving tests, cost cap and explicit benefit. Do not combine large unrelated refactors with features. Track accepted deferrals with a review trigger, for example repeated incidents or a dependent feature.

Suggested measures: repeated findings, verified escaped defects, maintenance minutes per accepted feature, new architecture violations and debt-related incident impact. Treat unmeasured debt estimates as estimates.

## Acceptance and stop conditions

- An existing finding becomes a linked debt item without duplicating task state.
- A planted architecture violation is caught by an independently reviewed check.
- An accepted exception has scope, owner, rationale and review trigger.
- Refactoring preserves independent behavior tests; no required gate is weakened.
- A debt intervention reduces observed friction or risk; stop speculative cleanup if it cannot demonstrate value.

## Agent follow-up

Link PR #344 for impact analysis, #345 for cost attribution and #337 for convergence. Use existing weekly audit and fix ladder; no new agent role or service is required for the first experiment.

## Record and evidence

Preserve this dated owner-requested proposal. Link later experiments, decisions and implementation PRs; do not silently delete or replace the source record. The living design remains `docs/design.md` and the canonical sources index is `docs/prior-art.md`.

**Checked:** [base 9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; design sections 5, 8, 9, 10, 11, 20, 23, plan, status, research index and open PR titles. **Inferred:** candidate gaps, priorities and experiment design. **Confidence:** medium; no live validation or full code audit. External links identified above were checked only to the extent stated.

## Decision brief — debt as observed maintenance cost, 2026-11-26

**Decision:** retain weekly audit, findings, fix ladder and baseline checks. Add a lightweight debt view only after correcting evidence integrity (A06). Prioritize a measured recurring consequence and a scoped intervention, not a generic cleanup agent. The baseline severity edge case below deserves an integration test before stricter rule promotion.

### Pinned implementation findings

| Evidence | Disposition |
| --- | --- |
| [Design §10](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/design.md#L673-L709), [audit skill](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/skills/own/audit/SKILL.md#L17-L32) | Existing design calls for weekly audit, root-cause analysis, cheapest intervention, planted regression checks and trial rules. Audit skill requests measurements and before/after evidence. Do not create another audit role. |
| [Recurring](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/learn/cluster.go#L41-L116) | Existing deterministic clustering counts distinct tasks, late catches and samples; groups by rule or category/first four words. It is a discovery heuristic, not a causal debt ledger. Input lacks tenant, repository, affected component, severity and cost; caller must isolate scope and enrich priority. Same rule may represent different causes; distinct wording may fragment one cause. |
| [Check.Run](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/check/run.go#L71-L108), [SARIF](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/check/sarif.go#L141-L191) | Existing new-violation multiset baseline. Repeated identical fingerprints are counted, but matching does not distinguish severity. A formerly tolerated warning becoming blocking with identical fingerprint can be suppressed by the helper; verify the actual gate and policy path. |
| [Standards checklist](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/standards/standards.go#L20-L50), [card resolution](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/standards/cards.go#L105-L145) | Existing active standards, precedence and overrides. Do not infer a complete trial-to-active enforcement lifecycle from card schema. |
| [ADR 0016](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/adr/0016-improvement-reports-per-tenant-and-per-operator.md#L18-L33), [phase 7](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/plan.md#L141-L143) | Counters-first tenant reporting is already accepted, nightly learning model half and taxonomy remain open. Debt measurement should reuse that path. |
| [A06](https://github.com/IlyaChakun/nightshift/pull/337) | Fixed-on-enqueue and inconsistent finding ingestion invalidate naive closure/recurrence metrics. Resolve first. |

### Primary research and limits

- [SEI, Managing Technical Debt in Software-Reliant Systems](https://www.sei.cmu.edu/library/managing-technical-debt-in-software-reliant-systems/), abstract excerpt, 2010: compromises can satisfy immediate needs while imposing later maintenance costs. [Nord et al., In Search of a Metric for Managing Architectural Technical Debt](https://www.sei.cmu.edu/library/in-search-of-a-metric-for-managing-architectural-technical-debt/), abstract excerpt, 2012: architecture-focused measurement supports strategic decisions. Neither validates a universal automated debt score.
- [Martin Fowler, Technical Debt Quadrant](https://martinfowler.com/bliki/TechnicalDebtQuadrant.html), page opened: prudent deliberate deferral is not the same as accidental poor design. Preserve explicit deferral rationale; not every defect is debt.
- [ArchUnit user guide](https://www.archunit.org/userguide/html/000_Index.html), opened: Java bytecode-based dependency/layer/cycle rules executed as tests. Transfer executable architecture contracts; use native Go tooling for this repo and existing Java pack for Java tenants only when approved. No ArchUnit dependency for Nightshift core.
- #344 documents native graph limitations; #345 defines cost provenance; #350 protects trial evaluation. Reuse them. Earlier NIST/DORA links above are background; not a claim of compliance or a debt-specific validated metric.

### Reproducible baseline edge case

Executed 2026-11-26 in a disposable Go 1.23.2 linux/amd64 program, with the exact NewViolations loop from sarif.go and a reduced Finding struct containing Fingerprint and Severity. No network or model calls.

| Baseline | Current | Observed NewViolations count |
| --- | --- | --- |
| one warn, fingerprint same-fingerprint | one block, same fingerprint | 0 |
| one warn, same fingerprint | two blocks, same fingerprint | 1 |

Thus multiset identity works for excess occurrence count but not severity escalation. This does not prove a production bypass: fingerprint generation, baseline recomputation, mandatory rule handling and gate call sites may change the actual outcome. Add a repository test covering tool-supplied identical partial fingerprint, rule/policy version and severity change; an existing approved debt baseline must not silently waive a newly mandatory check. Do not blindly make every historical warning blocking either; migration needs reviewed scope and explicit exceptions.

### Minimal debt contract (proposal)

A debt item is an owner-visible decision about an observed cost or architectural risk, not another task state. Link existing finding IDs, rule and accepted ADR/invariant, tenant/repository/component, causal hypothesis, observed consequence and evidence window. Record estimated remediation effort (principal), recurring additional effort/risk (interest), uncertainty, scope, owner, disposition and review trigger. Distinguish confirmed defects requiring immediate correction, intentional accepted exceptions, suspected debt, and optional aesthetic cleanup.

Use ranges rather than false precision. For recurring manual work, estimate avoidable minutes per occurrence from measured history and multiply by expected comparable occurrence count over a declared horizon. Compare with refactor + verification + migration cost in the same units. Monetary cost and risk remain separate; severe security/reliability constraints are non-negotiable floors, not discounted because frequency is low. Do not assume all recurring findings are independent or sum overlapping costs. A break-even estimate is a decision aid, not a guarantee. Example only: 20 minutes of observed repeated work on each of six comparable changes/month would imply about two hours/month; a four-hour repair would break even in about two months if recurrence and effectiveness hold. No real Nightshift measurements implied.

### Fitness functions and staged remediation

1. Start from an accepted invariant (e.g. worker cannot access control-plane DB; only approved path mutates release state; tenant's declared layer must not import adapter implementation). Trace to source/ADR. Choose the narrowest deterministic test, native dependency check or contract fixture. Existing constitutional and side-effect tests are precedents. A graph cannot prove runtime behavior; use negative tests for dynamic or cross-process boundaries.
2. Establish current violations and baseline provenance (tool version, scope, rule version, severity, exact revision, fingerprint). Plant a violation and an allowed neighboring change. Validate against full mandatory verification; characterize false positives and blind spots. Separate historical exceptions from unknown tool failures. Exceptions are specific, time/review-trigger bounded and owner-approved, never a broad ignore.
3. Test candidate check in warn-only trial where policy permits; do not weaken existing mandatory checks. Trial counts need exposure denominator, independently adjudicated true/false positives, severity and owner minutes. Zero detections does not prove uselessness when relevant exposure is zero. Activate/retire under design §9–10 and A02 authority; no self-approval.
4. Select one real recurring issue and compare (A) defer with trigger, (B) local refactor with behavior-preserving tests, (C) deterministic check or template change. Choose smallest intervention that addresses cause. Separate optional refactor from unrelated feature; allow emergency maintenance under existing authority. Record exact baseline/candidate and rollback, preserve source finding and negative result.

### Acceptance and stop conditions

- Repeated same-rule findings from separate root causes remain separately adjudicable; same defect with changed wording is not automatically new debt; grouping is a hint. Two tenants' samples and cost never mix.
- A new mandatory violation or severity escalation is not suppressed by a stale fingerprint exception; unknown/failed tool execution is not a clean result.
- Planted architecture breach is caught, permitted changes stay green, required behavior and tests are unchanged, rule provenance and exception expiration are visible.
- Compare measured rework, maintenance time, risk incidents/escapes, accepted outcomes and overhead over a fixed window; include costs of new checks and ongoing maintenance. A lower raw violation count alone is not success.
- Do not impose arbitrary complexity/LOC quotas or recurring blanket refactors. If no causal cost/risk reduction is demonstrated, defer or retire the intervention. Reuse audit/learn and one debt projection; add persistent lifecycle only if explicit deferrals cannot be represented in current decision/events.

**Checked:** pinned clustering, check/baseline, standards, audit and phase 7 paths; SEI abstracts, opened ArchUnit and Fowler pages; local Go helper experiment. **Inferred:** actual gate exposure, debt economics and proposed rule lifecycle extensions. **Confidence:** high for helper behavior, medium for integrated policy and economic estimates. **Not verified:** full trial wiring, production bypass, actual debt costs, integration tests, local make verify or CI.
