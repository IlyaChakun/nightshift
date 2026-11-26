# Nightshift — first real product pilot and measurement plan

**Proposed experiment:** 30 days of autonomous development. Time box and targets below are hypotheses, not a promise or industry standard. Start only when minimum safety prerequisites are met; extend or stop rather than bypass a gate. The objective is to learn whether Nightshift produces valuable, reliable software with less human engineering effort than a simpler baseline.

## Contents

- [1. Choose a useful but bounded product](#1-choose-a-useful-but-bounded-product)
- [2. Preflight and experiment charter](#2-preflight-and-experiment-charter)
- [3. Schedule and milestones](#3-schedule-and-milestones)
- [4. Fair baseline and evaluation design](#4-fair-baseline-and-evaluation-design)
- [5. What to monitor](#5-what-to-monitor)
- [6. Initial targets and go/no-go](#6-initial-targets-and-gono-go)
- [7. Instrumentation and dashboards](#7-instrumentation-and-dashboards)
- [8. Reporting cadence and templates](#8-reporting-cadence-and-templates)
- [Run / objective ID](#run--objective-id)
- [9. Controlled improvement and end-of-pilot deliverables](#9-controlled-improvement-and-end-of-pilot-deliverables)

## 1. Choose a useful but bounded product

Select one problem from real potential users, ideally 3–5 conversations or observed workflows before committing. Use an independent tenant and repository, unrelated to an employer or customer-confidential system. Avoid regulated or high-risk data, irreversible actions, payment handling and privileged production systems in the first pilot. Choose a product with a UI/API, persistence, a few dependent features, deployment, observability and ongoing feedback. Example candidates: lightweight recurring-task tracker, document checklist workflow, or personal research organizer with synthetic fixtures. Prefer whichever solves a real observed problem; do not choose a demo just to inflate factory throughput.

Define one target user segment, a recurring job, a baseline workaround, value hypothesis and a measurable user outcome. A small real user group is better than a large artificial benchmark masquerading as market demand. Synthetic data and test users are appropriate for infrastructure validation but not evidence of demand.

## 2. Preflight and experiment charter

Before starting, pin Nightshift commit, harness/model versions, policy revision, product spec revision and evaluation suite; record known missing capabilities. Create isolated staging infrastructure, sandbox credentials and non-sensitive data. Give the factory only explicit permissions. Require fail-closed verification, artifact-bound release authorization, tenant/worker isolation, bounded retry and cost budgets, a kill switch, backup/restore and rollback. Reconcile audit NS-A01 through A04 with HEAD; unresolved critical defects block affected operations. Public deployment is a separate decision.

Example charter (fill in with real values):

```yaml
pilot_id: ns-pilot-001
product_id: TBD
tenant_id: TBD
factory_commit: TBD
hypothesis: Nightshift delivers verified useful features with less human effort
primary_engineering_metric: human_minutes_per_accepted_objective
primary_product_metric: successful_target_workflow_per_eligible_user
observation_window: 30d
budget_usd: TBD
max_concurrent_jobs: TBD
release_policy: staging_first_owner_approved_production
baseline: same_agent_without_Nightshift
holdout_owner: independent_evaluator
stop_conditions: [security_violation, unauthorized_release, budget_exhausted,
                  unrecoverable_data_loss, unacceptable_quality_regression]
```

No hand-written product code is a useful experimental constraint, not the main success metric. Human safety intervention is always permitted; log it and count the affected objective correctly. Do not hide unsuccessful attempts, abandoned work, approvals or time spent maintaining the factory.

## 3. Schedule and milestones

| Phase | Actions | Exit evidence |
|---|---|---|
| Days 1–7: stabilize and baseline | Safety checks, representative small real task runs, independent scenarios, cost capture, baseline runs, product discovery | A versioned charter, tested recovery and negative authorization checks, agreed product intent |
| Days 8–14: deliver MVP | Lead decomposes approved objective, Builder implements, Verifier checks tasks and integration, staging preview and release | Integrated acceptance tied to exact artifact; usable MVP; all intervention/cost logs |
| Days 15–21: operate | Invite approved pilot users, measure outcomes, handle feedback and defects, test rollback and operational alerts | Real usage evidence, tracked issues, tested recovery and maintenance changes |
| Days 22–30: learn | Analyze failures, prioritize one or two improvements, compare candidate factory on frozen baseline and holdout, canary only if safe | Comparative report, documented limitations, promote/reject/rollback decision and next product roadmap |

If a phase cannot satisfy its exit conditions, record why and adapt scope or time box. Do not bypass verification or force production merely to meet a date.

## 4. Fair baseline and evaluation design

Compare Nightshift against the same underlying coding harness/model without Nightshift orchestration on a representative frozen set of tasks. Give both the same task intent, repository starting state, allowed tools, compute/time/cost limits, and independently held acceptance. Use separate branches/workspaces, randomize assignment/order if possible, and run multiple samples to reduce model variance. Compare equal complexity cohorts; report model/provider changes and failures. Do not count implementation of the baseline's test scaffolding as Nightshift-only work unless actually required. Include an optional conventional human-assisted baseline if feasible and report the distinction.

Separate two evaluations: (a) paired task-level comparison, (b) real product lifecycle where isolation, scheduling, recovery, coordinated acceptance, operations and human attention can create additional value. A standalone coding agent may outperform Nightshift for simple tasks; that is useful data. Include task/feature level, actual usage, and total cost of ownership. Use confidence intervals or distributions where sample size supports them; otherwise report raw counts and limitations. Never claim causation from an uncontrolled before/after product metric alone.

## 5. What to monitor

### Engineering effectiveness

| Metric | Definition |
|---|---|
| Autonomous completion | Verified accepted objectives with no unplanned human intervention / all eligible started objectives; report policy approvals separately |
| First-pass acceptance | Accepted on first full candidate / objectives evaluated; missing candidates are reported separately |
| Human attention | Actual minutes of specification, approvals, review, rescue/debugging, operations and factory maintenance; break down by type |
| Time to verified delivery | Approved objective start to independent acceptance and separately to release; median/p90, queue time and abandoned/censored work |
| Defect escape rate | Accepted objectives with confirmed escaped defects / accepted objectives with comparable observation windows; also report severity |
| Rework | Repair attempts/time/cost per objective and repeated finding rate |
| Requirement coverage | Independently satisfied required criteria / applicable criteria; mandatory failures always block |
| Cost per accepted objective | Model/API + infrastructure + evaluation costs across all attempts / accepted objectives; report human time separately and optionally price it |

### Platform reliability and safety

Track jobs queued/running/stalled, queue age, retries, deduplication, outbox lag, worker crashes, recoveries, recovery time, duplicate side effects, state-transition failures, sandbox violations, stale evidence/approvals rejected, release and rollback results. Track token consumption, rate limits, model latency, budget utilization, resource saturation and provider failure. Security events must not be sampled away. A count of zero observed incidents does not establish safety without negative testing and sufficient exposure.

### Product outcomes

Choose one primary metric for the user problem and 2–3 guardrails, e.g. successful first workflow within 24h, time to complete that workflow, repeated use in a defined period; guardrails include error rate, privacy incidents, support burden and latency. Record eligible cohort, denominator, metric revision, observation window, baseline, release/feature flag version, feedback themes and confounders. Report user sample size and uncertainty. With low traffic, prioritize direct user interviews, observed usability tests and qualitative evidence over misleading A/B test claims. Track whether the team stopped or changed low-value work.

## 6. Initial targets and go/no-go

These are candidate engineering goals to calibrate after baseline, not measured performance:

| Measure | Candidate threshold |
|---|---|
| Real eligible engineering tasks | Aim for 50+ across representative complexity, without splitting work artificially |
| Autonomous accepted completion | >=80%, with unplanned intervention tracked separately |
| First-pass acceptance | >=60% |
| Production/staging releases | Aim for 10 meaningful releases; do not create unnecessary releases to hit target |
| Unauthorized release, cross-tenant leak, lost durable task | 0 tolerated; verify adversarial and crash cases |
| Human-written product code | Aim for 0; disclose exceptions |
| Human effort / cost per accepted objective | Improve versus matched baseline or justify overhead by measurable reliability/quality gain |
| Product usefulness | At least one owner-defined workflow outcome demonstrated with real users; evidence quality reported |

Expand only if required safety gates hold and quality, human effort and cost show a credible improvement or clearly justified trade-off. Pause on inconclusive evidence; stop/revoke access on safety violations. A useful negative result might show Nightshift is only worth using for multi-task or long-running workflows.

## 7. Instrumentation and dashboards

Reuse existing OpenTelemetry/Grafana, event history, job state and cost/budget mechanisms if current HEAD supports them. Add missing events/fields instead of creating a second observability stack. Use structured events for objective, task, attempt, verification, intervention, deploy, rollback, outcome and factory candidate promotion. Store sensitive details in protected scoped records, not metric labels, shared prompts or public artifacts.

Illustrative event envelope (not current schema):

```json
{"event":"verification.completed","event_id":"uuid","occurred_at":"RFC3339",
 "tenant_id":"...","product_id":"...","objective_id":"...","task_id":"...",
 "run_id":"...","attempt":1,"factory_version":"...","model_version":"...",
 "spec_revision":"...","artifact_digest":"...","policy_version":"...",
 "scenario_suite_hash":"...","outcome":"passed","duration_ms":1200,
 "cost_usd":0.12,"human_intervention":false}
```

Use stable IDs and idempotent ingestion; events that authorize transitions must be durable, not best-effort telemetry. Correlate traces and records with task/run IDs; do not use unbounded IDs as metric labels. Aggregate labels by low-cardinality phase/status/model/profile; keep tenant-scoped access in dashboards and queries. Reconcile provider billing with internal cost estimates and distinguish unknown from zero. Retain enough evidence to reproduce outcomes while respecting tenant retention/deletion policy.

Recommended views:
- **Owner overview:** accepted vs attempted objectives, cost, human minutes, blockers, product outcome and guardrails.
- **Factory operator:** queue age, state counts, retries, stalled tasks, provider/worker/DB health, cost burn, release and security signals.
- **Quality:** acceptance funnel, failure categories, evidence validity, escaped defects and recovery drills.
- **Learning:** version comparisons, improvement hypotheses, holdout results, canary and rollback history.

Alert immediately on suspected unauthorized actions or leakage and disable affected automation; alert on budget exhaustion before starting more work. Define tenant-specific alert thresholds for queue age, repeated failures, outbox lag and product SLOs based on baseline. Deduplicate alerts and provide actionable evidence. A daily digest should show what needs attention, not every routine success.

## 8. Reporting cadence and templates

Daily: check safety and budget, review stuck work and intervention requests, record product feedback and exceptions. Do not manually rescue silent failures without an intervention entry. Weekly: compare cohort metrics with baseline, inspect escaped defects and root causes, review product hypotheses and guardrails, choose the next highest-value improvement. At pilot end: publish report and explicit continue/adjust/stop decision.

Suggested run record:

```markdown
## Run / objective ID
- Factory/model/policy/spec versions:
- Product hypothesis and acceptance:
- Started/finished/released timestamps:
- Result: accepted / rejected / blocked / cancelled / abandoned
- Independent evidence and exact artifact:
- Attempts, cost and infrastructure time:
- Human interventions (reason, minutes, action, policy approval vs rescue):
- Failures, recovery and escaped defects:
- Product outcome (window, eligible cohort, baseline, limitations):
- Learning candidate / follow-up decision:
```

Suggested weekly report: cohort counts and exclusion reasons; autonomy/acceptance/time/cost/human time; safety and recovery outcomes; product outcome and qualitative feedback; comparison with baseline; top recurring failure classes; next hypotheses and budget. Link raw evidence; include negative results.

## 9. Controlled improvement and end-of-pilot deliverables

Create a frozen evaluation set, separate hidden holdout, and candidate improvement record. Test candidate N+1 against trusted N, compare cost and quality, require independent promotion authority and verify rollback. Do not let the candidate modify its own evaluation. Track whether an improvement transfers to new tasks rather than memorizing the pilot. A controlled change that is rejected is still useful evidence.

Deliver: reproducible experiment charter, product repository and deployable verified artifact, run/intervention journal, operational dashboard, evaluation dataset (sanitized), baseline vs Nightshift report, product feedback and outcome report, proposed factory improvements with actual test results, and a decision on what to build next. Keep raw sensitive data outside public repositories. Use [03-autonomy-vision.md](03-autonomy-vision.md) to update maturity based on demonstrated capability rather than activity volume.

## Repository reconciliation and retention

This dated external research proposal is not a normative design change. Preserve the snapshot and append links to subsequent decisions and evaluation evidence rather than deleting it when superseded. Current status calls for live Lead, deployment, acceptance, multi-tenant and worker recovery checks. Align pilot prerequisites with those checks. Spending caps, new egress, credentials and irreversible actions require owner authorization.

**Checked:** repository rules, status and research index at [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26. External source dates and links are listed in the research pack. **Inferred:** proposed architecture, experiments and maturity targets; no implementation or runtime validation. **Confidence:** medium.
