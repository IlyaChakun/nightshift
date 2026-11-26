# Production intelligence and autonomous operations

Close the loop from deployed software to evidence-backed maintenance tasks and verified recovery. Begin with read-only incident triage and a synthetic product; keep production mutations behind the existing authority and release boundaries.

**Priority:** P1/P2. **Status:** research proposal, not approved implementation.

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
- [Decision brief — incident-to-recovery loop, 2026-11-26](#decision-brief--incident-to-recovery-loop-2026-11-26)

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

## Decision brief — incident-to-recovery loop, 2026-11-26

**Decision:** distinguish factory health, product reliability and product value. Build one bounded, read-only incident workflow on the existing intake, book, support skill and deployment authority. Prove independently observed recovery in staging before automating mitigation. No general-purpose SRE agent, production shell or new observability platform.

### Pinned implementation and dispositions

| Evidence | Implication |
| --- | --- |
| [Factory health](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/summary/health.go#L20-L58) reads code-scanning alerts, idle PRs and default-branch failures; unknown reads are explicitly rendered unknown. | Repository hygiene, not live product SLO monitoring. Retain the unknown/not-healthy distinction. |
| [Support skill](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/skills/own/support/SKILL.md#L12-L27) checks duplicates, expected behavior and fixes, reproduces in a worktree and discloses unreproduced failures. | Reuse Lead support for diagnosis and spec; do not invent a separate role or give it production write access. |
| [PR #328](https://github.com/IlyaChakun/nightshift/pull/328), inspected open diff and body | Proposed hourly alert/idle-PR tasks, cap of three per run, title-based lifetime deduplication and cancellation of queued resolved work. Reopening the same alert does not produce new work by design. This is a bounded maintenance proposal, not an incident lifecycle; reconcile rather than duplicate it. It has no independent off switch according to its PR body. |
| [Deploy runner](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/deploy/deploy.go#L168-L229) journals release, smoke and rollback; maps unsupported smoke to success. | Explicit compatibility behavior today. For a service whose policy requires behavioral verification, unsupported must be NOT_RUN, not PASS. After successful Rollback the runner returns RolledBack without a distinct independent recovery check. This is a concrete assurance gap, not evidence that every adapter restores incorrectly. |
| [Compose adapter](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/adapters/deploy/composelocal/composelocal.go#L229-L283) keeps a previous healthy revision; smoke checks environment state, optionally executes a project command; rollback invokes up. | Existing recovery mechanism; verify actual restored revision, mixed revision handling and user-facing health, not just command success. A container-level healthy state is weaker than product recovery. |
| [Task tracing](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/tracing.go#L14-L25) correlates task execution. | Extend correlation to tenant/service/environment/deployed artifact at an approved signal adapter; factory tracing alone cannot diagnose the tenant's production system. |

The existing status reports phase 6 on stubs and no real Docker validation. Do not label these findings reproduced in production. A01 governs mandatory gates; A02 stable factory promotion is separate from product rollback; A03/A04 govern durable actions and isolation.

### Industry evidence and transfer

Primary pages opened 2026-11-26:

- [OpenAI Performance Factory, 2026-11-19](https://openai.com/index/how-openai-performance-factory-keeps-chatgpt-fast/): human assessment of 100 proposed diagnoses rated 70% accurate, with the remainder incomplete or needing refinement. A separate review tries to disprove the causal explanation; engineers review fixes; the workflow checks recovery after deployment. The article also reports fixes are harder than diagnoses. Transfer the independent challenge and recovery loop, not its scale or the assumption that every incident can be repaired autonomously.
- [Google SRE: Alerting on SLOs](https://sre.google/workbook/alerting-on-slos/), [error budget policy](https://sre.google/workbook/error-budget-policy/) and [incident response](https://sre.google/workbook/incident-response/): alert on user-visible reliability, prioritize according to impact, coordinate response and mitigate before deeper root-cause work. The example policy is illustrative, not a universal SLO or change-freeze threshold.
- [Prometheus Alertmanager](https://prometheus.io/docs/alerting/latest/alertmanager/): grouping, deduplication, routing, inhibition and silencing address notification floods. Reuse these concepts or an existing tenant alert source; no new Alertmanager deployment required for the synthetic pilot.
- [Google SRE: Canarying releases](https://sre.google/workbook/canarying-releases/): compare limited exposure with a reference and use measured signals to decide rollout. Canary statistics need sufficient representative traffic; a small local product may need synthetic probes instead.
- [DORA](https://dora.dev/guides/dora-metrics/): distinguish failed-deployment recovery from all-incident restoration. Record actual delivery instability and recovery separately; do not optimize a single MTTR number.

### Minimum incident contract (proposal)

Separate three identities: transport delivery ID (replay deduplication), stable condition identity (tenant, authorized source, service, environment, rule/fingerprint), and incident occurrence ID (a new firing episode after verified closure). Do not use mutable display titles as the authority. Correlate symptom groups without merging unrelated incidents. Use explicit source status (firing/resolved/unknown), timestamps and a monotonically ordered update or source version when available; reconcile out-of-order events with a fresh authenticated read. A resolved notification does not itself prove recovery.

One proposed durable incident record references the above IDs, actual deployment revision/artifact, first/last seen, severity and SLO impact, evidence window, investigation budget, linked task IDs, diagnostic hypotheses, recovery criterion, disposition and independent evidence. Keep only necessary redacted data. Source authentication and tenant/service allowlist precede enqueue; source-provided text, links and labels are data, never executable instructions. Store sensitive details outside public records. Unknown/failed ingestion preserves current incident state.

Proposed states: OPEN → TRIAGED → INVESTIGATING → DIAGNOSIS_REVIEWED → REPAIR_LINKED → OBSERVING → RECOVERED; BLOCKED/ESCALATED and REOPENED are explicit dispositions. These are logical milestones, not new factory task states. Begin with the existing task and incident evidence ledger; add a first-class incident record only if multiple signals/tasks/recurrences prove necessary. Incident resolution must not be inferred from task state released or merged. A rollback can mitigate an incident while root cause and permanent remediation remain open.

### First experiment and authority

1. Use the first pilot's isolated staging service with synthetic data and a manual or simulated authenticated alert source. Define one user-visible SLI (e.g. good request fraction), SLO, minimum sample count, observation window, independent probe and runbook before starting. Burn rate is observed bad-event fraction divided by the allowed bad-event fraction; no traffic or unavailable telemetry is unknown, not zero burn. Select short/long windows and alert thresholds for this service, not copied production defaults.
2. Seed a reversible regression in one known revision. Send duplicate and out-of-order signals; preserve one incident occurrence and one active investigation. Read the deployed revision, baseline, recent changes, redacted metrics/logs/traces through bounded read-only access. Use the support skill to reproduce; record alternative causes and confounders, including a negative control or rollback comparison when feasible.
3. Independent Verifier challenges diagnosis before a fix is proposed: evidence, causal mechanism, disconfirming test, uncertainty. If insufficient, return once within budget or escalate. Do not grant incident severity authority to execute arbitrary mutations.
4. Route a confirmed repair through existing spec, Builder, Verifier, preview and release gates. Existing owner policy decides release. An initial mitigation may be the existing authorized rollback of a known compatible revision; never auto-rollback irreversible data migrations. Keep action idempotency, attempt and authority evidence. Before reporting rollback successful, independently inspect target revision and health; if unavailable report recovery unverified and escalate.
5. Verify the original failing behavior and agreed guardrails through an independent signal over the predefined window and sufficient samples. If the signal is unavailable or the service has mixed/unknown revision, remain observing/blocked. Partial recovery records residual impact; failed recovery reopens within a strict budget, then escalates. Preserve mitigation/fix/incident times separately.
6. Compare manual baseline to assisted read-only triage on matched synthetic scenarios. Track all false alerts, skipped and unresolved incidents, wrong diagnoses, operator minutes, active cost, time to detect/triage/mitigate/verified recovery, recurrence and change-induced regressions. No live customer data, paid calls, new credentials or egress without approval.

### Failure matrix and go/no-go

| Planted case | Required result |
| --- | --- |
| Replay, burst, concurrent duplicate, restart | Bounded work, one logical investigation per occurrence, durable dedupe; no duplicate mutations |
| Condition resolves then recurs | New occurrence or explicit reopen linked to history; not suppressed forever by old task title |
| Signal unavailable, out-of-order resolved, zero samples | No false closure; reconcile or escalate |
| Untrusted log text or foreign tenant identity | No privilege change, unauthorized read, new egress or action |
| Plausible wrong cause | Independent challenge rejects or marks uncertain; no confident unsupported diagnosis |
| Required smoke unsupported, restored service unhealthy, wrong revision | NOT_RUN/failed recovery, not verified success; stop autonomous progression |
| Non-reversible migration or rollback failure | No blind rollback/retry; owner escalation, preserve evidence |
| Healthy unrelated service and noisy non-actionable alert | No unsolicited repair or unbounded cost |

Proceed from manual intake to automatic read-only triage only if the above safety/identity cases pass and observed attention or diagnosis benefit justifies it. Bounded automatic mitigation is a later, separately authorized step with allowlisted reversible actions, cooldown, independent health check, kill switch and cost cap. Stop if recurring false alarms or wrong diagnoses outweigh benefit. Product feedback and feature discovery belong to #340, not an incident escalation path.

**Checked:** pinned source and open #328 diff/body, status, primary sources above. **Inferred:** incident contract, state mapping and priorities. **Confidence:** high for static smoke/rollback semantics and documented source behavior; medium for incident economics. **Not verified:** live adapter, effective credentials, actual recovery or diagnosis, multi-conductor dedupe, local tests and CI.
