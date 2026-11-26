# Production intelligence and autonomous operations

Close the loop from deployed software to evidence-backed maintenance tasks and verified recovery. Begin with read-only incident triage and a synthetic product; keep production mutations behind the existing authority and release boundaries.

**Priority:** P1/P2. **Status:** research proposal, not approved implementation.

## Problem

A successful release is not evidence that a service stays healthy or meets user needs. Alert floods, duplicates and untrusted logs can create expensive remediation loops. An agent with unrestricted production access can cause a larger outage than the original fault.

## How it works

Connect allowlisted, tenant-scoped operational signals to the existing intake path. Normalize events into a proposed incident record with service, environment, deployment revision, severity, time window, SLO impact, signal references, deduplication key, ownership and confidence.

Correlate incidents with recent releases and existing tasks, gather read-only evidence, rank candidate causes and reproduce in an isolated environment. Lead proposes a repair; Builder and Verifier use the existing delivery flow. After authorized release, measure recovery over a predefined window against an independent signal. Close, reopen or escalate based on evidence.

Keep incident response distinct from product analytics: an operational alarm can justify a repair proposal, not unbounded permission to implement unrelated features.

## Trade-offs

| Option | Benefit | Limitation |
| --- | --- | --- |
| Manual alert-to-task intake | Safe, simple baseline | Operator overhead |
| Automatic deduplicated read-only triage | Faster diagnosis | Noise, incomplete signals |
| Bounded automated mitigation | Faster recovery for known cases | Requires independent safeguards |
| General autonomous production shell | Broad capability | Excessive blast radius; reject |

Choose read-only diagnosis first. Automatic mitigation should be limited to predefined, reversible actions with explicit policy, rate caps and independent health checks.

## Sources

- [Google SRE example error budget policy](https://sre.google/workbook/error-budget-policy/), search excerpt checked 2026-11-26: operational decisions should respond to measured reliability budgets. Treat its example thresholds as examples, not Nightshift defaults.
- OpenAI Performance Factory in PR #339: previously discussed industry case; review its evidence and scope before transferring mechanisms.
- [DORA delivery metrics](https://dora.dev/guides/dora-metrics/), search excerpt checked 2026-11-26: recovery and deployment instability are relevant outcome measures.

## Fit

Design already covers watchdog, support, release, smoke, rollback and observability. PR #328 (open at the base) concerns alert and idle-PR tasks. Reconcile with that work. Product telemetry belongs to the tenant; core remains generic. No direct production credentials should be given to Builder.

## Status

Partial foundation: phase 6 deployment and rollback are reported on stubs; live acceptance and real engine validation remain open. A closed-loop real incident exercise is not established by the current status.

## Proposal

Use the first pilot product and a synthetic alert source. Define one SLO, a small runbook, an allowed action list and a fixed investigation budget. Inject a reversible defect in an isolated environment; run the full loop from alert to independently measured recovery.

Store source IDs and redacted evidence, not raw user data. Use authenticated signal ingestion, deduplication, backoff, incident suppression during known maintenance, and bounded escalation. Treat logs and error messages as untrusted data; never follow embedded instructions.

Use existing deployment authority, a kill switch, rollback and human escalation. No widening of egress, spending or credentials without owner approval.

## Acceptance and stop conditions

- Duplicate or replayed alerts create one incident/task; bursts do not exhaust the budget.
- A planted fault yields evidence-backed diagnosis or honest escalation.
- Untrusted operational text cannot change permissions or trigger an unauthorized mutation.
- Recovery uses independent measurements tied to the released revision and a defined observation window.
- If recovery fails, existing rollback/escalation rules apply; no endless remediation loop.
- Record detection, diagnosis, verified recovery time, false alerts, human minutes and change failures.

## Agent follow-up

Finish live phase 6 and the pilot prerequisites. Check PRs #328, #332, #338, #339 and #342 before implementing. Start with one signal adapter and one reproducible incident, not a general SRE platform.

## Record and evidence

Preserve this dated owner-requested proposal. Link later experiments, decisions and implementation PRs; do not silently delete or replace the source record. The living design remains `docs/design.md` and the canonical sources index is `docs/prior-art.md`.

**Checked:** [base 9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; design sections 7, 10, 14, 15, 16, 17, 18, plan, status, research index and open PR titles. **Inferred:** candidate gaps, priorities and experiment design. **Confidence:** medium; no live validation or full code audit. External links identified above were checked only to the extent stated.
