# Human attention and operator experience

Treat operator attention as a scarce resource and reduce avoidable decisions without weakening authorization. Evolve the existing Slack, CLI, MCP and digest interfaces around a consistent, evidence-backed decision queue.

**Priority:** P1. **Status:** research proposal, not approved implementation.

## Problem

An autonomous factory can produce more questions, alerts and reviews than its owner can process. Fewer messages can also be misleading if the system silently hides risk, discards feedback or auto-approves work without permission. Optimize decision quality and time, not message count alone.

## How it works

Classify operator interactions as mandatory authorization, missing product intent, unresolved technical trade-off, informational notification or duplicate. Keep one canonical decision record across all doors. Each actionable card shows what decision is required, why it matters, available options, recommendation with evidence, risk, reversibility, deadline if real, and the exact affected revision.

Group informational updates in a digest. Escalate only genuinely urgent events according to the existing policy. Recover known facts from verified project memory before asking the owner again. Never reinterpret silence as approval outside explicitly authorized policy.

## Trade-offs

| Option | Benefit | Limitation |
| --- | --- | --- |
| Current task threads and digest | Low complexity | Context fragmented across tasks |
| Ranked decision queue and reusable templates | Reduces switching and duplicate asks | Ranking errors may hide urgent work |
| Dedicated web console | Richer views and diagnostics | Extra surface, authentication and maintenance |
| Automated decisions for low-risk classes | Saves attention | Requires explicit scope and evidence |

Start with the existing doors and digest. A web console is justified only by measured operator friction. Do not optimize attention by loosening safety controls.

## Sources

- [DORA software delivery metrics](https://dora.dev/guides/dora-metrics/), search excerpt checked 2026-11-26: delivery outcomes should accompany activity measures. Human attention metrics here are original proposals, not DORA metrics.
- Prior research PR #342: pilot measurement and human intervention tracking.
- Design section 15 and `docs/prior-art.md`: existing operator, review and autonomy concepts.

## Fit

Design section 15 specifies a Slack Home panel, approval queue, switches, diagnostics and quiet hours. Section 13 covers project memory and section 10 treats operator comments as findings. Plan I7, S2 and existing digest work already cover several prerequisites. Extend them; keep Slack, CLI and MCP actions equivalent.

## Status

Partial: digest and task timeline are reported built; live timeline checks and parts of the operator experience are pending. An attention budget and decision-quality evaluation are candidate extensions, not confirmed omissions in code.

## Proposal

For the first product pilot, collect interaction IDs, reason class, timestamps, decision outcome, active human minutes when voluntarily measured, revision, risk and whether a verified prior answer existed. Avoid collecting private conversation contents merely to measure attention. Report estimates and missing values explicitly.

Compare the baseline with a consolidated digest and ranked decision cards. Use a small set of scenarios for a critical approval, repeated question, expired decision, conflicting user edits and a high-priority incident. Do not sample only accepted tasks.

Suggested measures: human minutes per accepted outcome, unnecessary questions, repeated decisions, median and tail decision wait, mandatory approvals missed, owner correction rate and operator satisfaction. A lower interruption count is useful only if safety and completion are maintained.

## Acceptance and stop conditions

- A required approval remains visible until resolved; severity cannot be downgraded by a summary agent.
- Duplicate notifications can be grouped without losing independent evidence.
- Stale approval cannot apply to a changed artifact, and decisions are consistent across doors.
- An urgent incident bypasses quiet-hour batching only according to the existing policy.
- Compare measured attention and decision latency against baseline; retain no optimization that increases unsafe or incorrect decisions.

## Agent follow-up

Coordinate with PRs #332, #340 and #342. Reuse the decision and digest records before proposing new storage. Keep the operator in control of risk, budget and product strategy.

## Record and evidence

Preserve this dated owner-requested proposal. Link later experiments, decisions and implementation PRs; do not silently delete or replace the source record. The living design remains `docs/design.md` and the canonical sources index is `docs/prior-art.md`.

**Checked:** [base 9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; design sections 6, 7, 10, 11, 15, 20, plan, status, research index and open PR titles. **Inferred:** candidate gaps, priorities and experiment design. **Confidence:** medium; no live validation or full code audit. External links identified above were checked only to the extent stated.
