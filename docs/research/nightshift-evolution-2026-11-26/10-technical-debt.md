# Technical debt and architecture evolution

Turn recurring maintenance findings into prioritized, justified architecture work. Track the cost of debt and enforce only proven architectural invariants; avoid a permanent autonomous refactoring treadmill.

**Priority:** P2. **Status:** research proposal, not approved implementation.

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
