# Research knowledge base

Techniques from outside systems, one entry each, and how far the factory has them. Systems and links live in `docs/prior-art.md`; this folder holds techniques. Entries: `factory-techniques.md` (orchestration, scenarios, twins, memory, safety) `durable-execution.md` (what durable execution needs and who provides it) and `harness-control.md` (how to control any harness from outside). The decision on Temporal is ADR 0018, with the experiment in `temporal-spike.md`.

## Entry format

Every technique has these lines, in this order.

- **Problem**: what goes wrong without it.
- **How it works**: two to four sentences.
- **Trade-offs**: benefits, limits, costs.
- **Sources**: links with the date read; mark proven practice (seen in production use or a stable spec) or experimental (a blog post, a draft spec, a demo).
- **Fit**: how it applies to the factory.
- **Status**: implemented, partial, missing or not applicable, with the file, section or plan item.
- **Proposal**: the smallest next step, or none.

A claim that came from a page we only skimmed carries **unverified**. Status was read from `docs/design.md`, `docs/plan.md`, `docs/status.md` and package listings on 2026-11-26; it is not a code audit.

## Gap table (2026-11-26)

| Technique | Status | Where / next step |
|---|---|---|
| State survives a crash (book, journal, handoff) | implemented | design sections 3, 7, 12; ADR 0005 |
| Retries, backoff, limit parking | implemented | River, ADR 0012 |
| Idempotent side effects | partial | outbox keys exist; R2 lists every side effect and tests it |
| Timeout per step | partial | agent job budget exists; conductor steps have none: R1 |
| Outside-event liveness, fault injection | missing | X1, X2 (open PR #233) |
| Hidden scenarios (holdout) | partial | visibility rule and skill built, storage open (phase 5) |
| Twins of external services | partial | GitHub, Linear, Slack twins exist; none is validated against the live service |
| Satisfaction score over scenarios | not applicable | volume too small; design section 19 |
| Filesystem as memory, memory pages | implemented | artifacts volume, ADR 0005 |
| Pyramid summaries, MapReduce over many items | missing | not needed until Lead reads many documents; revisit with `ns search` |
| Non-interactive build after an interactive spec | implemented | Lead, then Builder (design section 4) |
| Gene transfusion, semport | not applicable | a Lead skill when a tenant asks; no core change |
| Goal gates, deterministic edge choice | implemented | gates in code (ADR 0003), not a graph file |
| Loop detection in an agent run | missing | R3; today only wall-clock and turn limits |
| Reward-hacking guard (protected paths, hidden tests) | implemented | policy constitution, T6 |
| Context handoff between sessions | implemented | handoff (ADR 0005) |
| Record live calls and replay them | missing | the `fake` harness is scripted by hand; E1 below |
| Trajectory scoring (tool use, not only the result) | missing | polygon grades results only; E2 below |
| Provider independence | implemented | `Harness` (ADR 0002) |
| Sandbox and egress isolation | implemented | ADR 0004, T6; per-tenant state (M3) open |
| One trace per task | implemented | OpenTelemetry (design section 3) |

New plan items: R1, R2, R3 (ADR 0018). E1 and E2 are proposals for phase 7; they need the owner's word because they change what the polygon stores.
