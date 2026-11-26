# Human attention and operator experience

Treat operator attention as a scarce resource and reduce avoidable decisions without weakening authorization. Evolve the existing Slack, CLI, MCP and digest interfaces around a consistent, evidence-backed decision queue.

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
- [Decision brief — operator attention without authority loss, 2026-11-26](#decision-brief--operator-attention-without-authority-loss-2026-11-26)

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

## Decision brief — operator attention without authority loss, 2026-11-26

**Decision:** build a reliable decision read model over the existing book and doors before investing in a new console or automated approvals. Separate mandatory authority from notification delivery and avoid optimizing message counts at the expense of informed consent. Address the approval binding gap with A01/A03 first.

### Pinned implementation findings

| Evidence | Disposition |
| --- | --- |
| [ADR 0006](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/adr/0006-chat-doors-inbox-cli-mcp.md), [doors.go](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/doors.go#L44-L112) | CLI/MCP already share task/decision handling; Inbox derives the newest actionable card from task events. It stops at any owner_message or latest non-actionable card. A reply or notification does not necessarily resolve mandatory authorization. |
| [Inbox adapter](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/adapters/chat/inbox/inbox.go#L36-L78) stores inbox.card events and deduplicates delivery IDs. | This is a delivery projection, not a canonical approval lifecycle. Slack parity is not established by these paths; the inbox adapter is explicitly a separate configured chat adapter. |
| [Doors.decide](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/doors.go#L219-L236) rereads the current task and supplies its version to Inbox.Handle. | A caller supplies task ID, not the version it actually reviewed. Approval of a newer unseen spec is a plausible stale-intent gap. |
| [Inbox.button](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/chat.go#L268-L305) compares nonzero card version before calling [Mover.Apply](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/flow/move.go#L25-L78). Mover reads the task again and CASes that fresh state, but Move has no caller-expected version. | Existing CAS is valuable, but the pre-check and commit are not one version-bound decision. A concurrent edit/reapprove race needs a deterministic integration test; no exploit or live race reproduced. A zero version bypasses the pre-check. |
| [Summary.Build](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/summary/summary.go#L21-L81) and [Needs you](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/summary/summary.go#L164-L200) select a fixed set of states, cap the display at five, and report the rest as a count. Blocked is absent from that waiting set, although [dependency handling](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/deps.go#L232-L276) requests owner action. Ready may progress automatically. | State alone is an imperfect proxy for a decision. An old low-risk item can crowd out an urgent item; the underlying queue must retain every pending authorization. |
| [Morning](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/morning.go#L50-L105), [QuietHold](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/quiet.go#L13-L40), [spec card](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/lead.go#L336-L370) | Daily outbox dedupe and quiet hours already exist. QuietHold batches specific card kinds; finished spec cards use CardBlocker, which is not held by that rule. Verify intentional urgency vs presentation kind; do not assume every blocker is an incident. |
| [Lead enqueue](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/lead.go#L201-L234), [Lead instructions](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/roles/lead.md#L31-L55), [human review](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/gate.go#L207-L249) | Prior memory, similar tasks, short questions and revision-aware GitHub reviews are existing foundations. Preserve these controls; don't add an agent that can dismiss required review. |

### External research

Source search excerpts checked 2026-11-26; full papers were not read in this pass.

- [Meyer et al., Software Developers’ Perceptions of Productivity, FSE 2014](https://www.microsoft.com/en-us/research/publication/software-developers-perceptions-of-productivity/): survey of 379 developers and observation of 11; perceived productive days involved fewer substantial interruptions, while observed switching could coexist with perceived productivity. Supports measuring interruption quality and perceived control, not a universal time penalty per notification.
- [Meyer et al., Today was a Good Day, TSE 2019](https://www.microsoft.com/en-us/research/publication/today-was-a-good-day-the-daily-life-of-software-developers/): 5,971 responses from Microsoft developers describe daily work and aversion to interruptions/meetings. Self-reports are not randomized evidence that a ranked inbox improves Nightshift.
- [Google SRE, Eliminating Toil](https://sre.google/workbook/eliminating-toil/) and [Being On-Call](https://sre.google/workbook/on-call/): avoid repetitive operational work and page only for immediately actionable events. Transfer the actionability principle; do not copy Google's team staffing or operational-time targets into a solo factory.
- #342 and #345 already define cohort accounting. Human attention is a distinct outcome; do not invent a dollar price for it or conflate time waiting for approval with active work.

### Minimum decision contract

Start with a projection over task states, events, existing decision rows, GitHub review state and outbox. If it cannot represent a pending action reliably, add a small durable decision record, not another task engine. The following names are proposed, not existing schema fields.

| Dimension | Rule |
| --- | --- |
| Identity | Stable decision ID, tenant, task/objective, purpose, expected task version, spec/candidate/policy identity, applicable actor scope. Multiple doors render the same identity. |
| Lifecycle | Pending, resolved, superseded, expired or blocked. A notification being delivered/read, an unrelated reply or a digest acknowledgement does not close authorization. Expiry means reissue/escalate, not approve. |
| Decision content | Exact question/action, alternatives including defer/reject, short recommendation and basis, consequence, reversible/irreversible scope, actual deadline if any, evidence link. |
| Priority | Deterministic authority/risk/urgency/dependency impact and age. Never allow a model summary to downgrade a mandatory decision; show total and an accessible full list. |
| Commit | Actor authorization, expected identity and state transition checked in one transaction; record action and result. Stale versions return a visible refresh requirement. A duplicate submission is idempotent, not a second authorization. |

Require explicit version/decision ID from the displayed CLI/MCP response (with a migration path for existing tools). Ensure Slack clicks, CLI and MCP all apply the same expected version at commit; reject ambiguous zero-version approvals unless an independently justified trusted path exists. Never translate a recommendation, a generic owner message or silence into consent. Retain existing remote MCP permission boundary; do not add release or settings authority.

Notification policy: urgent actionable incidents use the existing explicit policy, ordinary decisions batch during quiet hours, status-only updates digest, duplicates coalesce by decision identity, not text. Distinguish an acknowledgement from a decision. Provide one durable pending view even if Slack delivery fails; use paginated projection or query before a web UI. Rank high-impact blockers, preserve older items and report the backlog. An owner choosing to postpone a decision is not a failed approval.

### Evaluation protocol and acceptance

Compare baseline with the projection and version-bound decision cards on the same frozen scenarios; only then experiment with ranking or batching. Record decision ID, role, time opened/resolved, notification attempts/delivery, decision class, correction, independent reason labels and voluntarily recorded active minutes. No need to store private chat contents to measure duration; retain tenant scope and missing-data labels.

- Primary: total human active minutes across all started objectives / independently accepted objectives (#342/#345). Also report raw minutes including rejected work, median/p90 decision wait, interruption episodes, avoidable repeated questions, corrected decisions, backlog age and subjective operator burden. No zero denominator; distinguish active time from latency and overlapping activities.
- Critical scenarios: card v1 displayed then spec edited to v2; edit between check and CAS; same approval submitted through two doors; unrelated reply followed by pending approval; non-actionable notification after a decision; blocked dependency; delivery failure/restart; urgent incident during quiet hours; unauthorized actor; remote MCP attempts prohibited release. Verify pending count and complete listing from authoritative state.
- Required result: stale intent never changes a newer decision; no hidden mandatory approval; duplicate logical action applies once; no widened authority; current evidence is visible and a denied/stale action is distinguishable from success. Verify with Postgres and actual adapter tests, not just UI snapshots.

**Go/no-go:** fix confirmed version-binding and pending-view gaps first. Adopt prioritization/batching only when the paired pilot reduces avoidable attention or wait without correctness/authorization regression. If the existing digest and reliable pending view suffice, keep them; defer web console (ADR 0006) and policy-based auto-decisions. Any new automatic approval class requires explicit owner authorization and independent safety tests.

**Checked:** pinned source paths, ADR 0006 and status; primary search excerpts listed above. **Inferred:** race exposure, projection and experimental design. **Confidence:** high for inspected API/selection behavior; medium for real-world attention benefit. **Not verified:** live Slack/MCP, concurrent stale-approval reproduction, complete action inventory, local tests or CI.
