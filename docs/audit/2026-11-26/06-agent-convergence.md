# NS-A06 — Convergence, context engineering and corrective strategies

**Priority:** P2.  
**Status:** capability improvement; establish baseline and inspect existing supervisor, return counters, memory and budgets first.

## Problem

Retries and bounded iteration prevent infinite execution but do not distinguish progress from stagnation. The earlier audit identified existing supervisor and escalation mechanisms; it did not establish that they detect repeated semantic failures, regressions or conflicting requirements. Additional model calls can increase cost without improving the candidate.

## Proposed invariants

- Each iteration has durable evidence of what changed, why it ran and how it performed against stable acceptance criteria.
- Repeated attempts are bounded by wall time, tokens, cost and relevant operation limits; the agent cannot expand its own budget or relax required checks.
- A change of strategy is based on observable evidence and recorded; failure to converge yields a diagnosable terminal or blocked state, not false success.
- Memory and tool output are untrusted inputs; tenant/role scope and provenance must be respected. Untrusted content must not become policy or trusted instructions.

## Design options

| Option | Value | Trade-offs |
|---|---|---|
| Normalize finding identities, record pass/fail transitions, patch digests and attempt cost | Cheap progress/stagnation signals | Finding wording changes and nondeterministic checks can produce false signals |
| Deterministic strategy policy (retry, refresh context, rollback, alternate harness, replan, escalate) | Bounded and explainable | Requires good classification and thresholds; avoid hardcoding speculative cases |
| LLM-assisted diagnosis of repeated failure | Useful for semantic causes and spec ambiguity | Additional cost, nondeterminism and susceptibility to misleading context; cannot override policy |

Start with instrumentation and a small deterministic escalation policy; tune on observed failures. Track unresolved required findings, regression severity and independent acceptance results rather than treating raw finding count as a monotonic score. Preserve best known verified checkpoints, but do not allow partial success to authorize release.

## Context and memory

Inspect retrieval scope, freshness and token limits. Store source provenance, project and role ownership, confidence and revision; distinguish factual artifacts, decisions, hypotheses and untrusted text. Prefer targeted retrieval over ever-growing transcripts. Validate policy-critical facts against authoritative state. Quarantine untrusted tool/repository content from instruction channels. Do not persist secrets or hidden scenario details into Builder-accessible memory. Turn recurring failures into deterministic tools/tests where feasible (rules into tools, words last), not automatically into global prompt instructions.

## Candidate recovery strategy

Classify infrastructure failures separately from code defects, spec ambiguity and architectural failure. Retry transient infrastructure failures with backoff. For repeated defects, give Builder a targeted, redacted failure summary; refresh context or switch harness if evidence justifies it. For persistent architectural or requirement conflicts, return to Lead or request an authorized clarification. Roll back regressions to a known checkpoint. Stop on budget exhaustion or policy violations.

## Acceptance criteria

Construct reproducible cases: same finding repeated with changed wording; genuine incremental progress; alternating patches; regression after a partial pass; conflicting requirements; transient provider failure; stale/poisoned retrieved text; exhausted budget. Verify appropriate strategy, bounded cost, durable reason codes and no relaxation of acceptance. Measure accepted completion rate and cost before/after policy changes on fixed representative tasks. Do not adopt more elaborate planning or routing unless it improves the baseline.

**Dependencies:** NS-A03 attempt identity/durability; NS-A05 parent replanning; NS-A07 benchmark and holdout strategy.

## Follow-up source check and retention

This dated external review is a candidate finding record, not the living design. Preserve the original findings and append a disposition or link to implementation evidence; do not delete this record merely because an issue is resolved. Superseding or relocating it requires an explicit owner decision.

**Checked:** pinned Nightshift base [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; `docs/status.md` and `docs/research/README.md`. Current status lists ADR 0019 supervisor and model/effort per role; research README already identifies trajectory scoring and replay as gaps. Build on these, do not duplicate them.

**Inferred:** priorities and proposed solutions; no tests or live runs performed. **Confidence:** medium.
