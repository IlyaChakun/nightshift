# Tenant-scoped product intelligence

**Status:** proposed design, not an assertion about current implementation. **Goal:** allow each tenant to configure how its products discover problems, prioritize work, validate hypotheses and learn from users, without making the engineering execution engine responsible for unrestricted product decisions.

## Contents

- [1. Product thinking is a feedback loop, not a PM prompt](#1-product-thinking-is-a-feedback-loop-not-a-pm-prompt)
- [2. Scope and proposed data model](#2-scope-and-proposed-data-model)
- [3. Example configuration (illustrative, not current Nightshift syntax)](#3-example-configuration-illustrative-not-current-nightshift-syntax)
- [4. Role design and decision authority](#4-role-design-and-decision-authority)
- [5. Evidence and discovery](#5-evidence-and-discovery)
- [6. Integration plan for Nightshift](#6-integration-plan-for-nightshift)
- [7. Acceptance and evaluation](#7-acceptance-and-evaluation)
- [Research leads (not yet verified for this package)](#research-leads-not-yet-verified-for-this-package)

## 1. Product thinking is a feedback loop, not a PM prompt

Delivery asks whether a feature meets its specification. Product thinking asks whether the feature is worth building and whether it produces the intended outcome. A product agent should be able to propose doing nothing, running an interview, changing documentation or removing a feature instead of generating code. Commercial demand and customer preferences are not inferable from model confidence alone.

```
Tenant goals + product strategy + constraints
    -> evidence (feedback, interviews, telemetry, support, incidents)
    -> opportunity / problem hypothesis
    -> options: discover, do nothing, experiment, build, remove
    -> prioritize and authorize within tenant policy
    -> versioned objective / specification / independent acceptance
    -> existing Nightshift delivery workflow
    -> observe product outcome + guardrails
    -> retain / iterate / rollback / stop
    -> update product knowledge and proposed roadmap
```

Keep this loop separate from the factory-learning loop in [03-autonomy-vision.md](03-autonomy-vision.md). Product learning changes what is worth building for one product. Factory learning changes how Nightshift delivers reliably. Raw tenant feedback must never automatically become global factory memory or instructions.

## 2. Scope and proposed data model

Tenant is the authorization/billing boundary. Product is the strategy and lifecycle boundary: a tenant may own multiple products, each with a distinct audience and economics. Repository is an implementation surface; one product may span repositories. Do not assume 1:1 tenant/product/repository. If the current schema uses a different model, preserve existing isolation and extend incrementally.

| Record | Required information / behavior |
|---|---|
| Tenant product policy | Tenant ID, authorized owner, data sources, autonomy ceiling, budgets, actions requiring approval, retention and sharing rules |
| Product profile (versioned) | Product ID, tenant ID, objective, audience/segment, problem, value proposition, non-goals, roadmap themes, constraints, metric definitions, strategy revision |
| Evidence item | Source ID/type, tenant/product scope, observed time, ingestion time, provenance, confidence, consent/retention classification; dedupe key |
| Opportunity | Problem statement, affected segment, linked evidence, alternatives, estimated impact, uncertainty, lifecycle state |
| Hypothesis / experiment | Proposed change, predicted outcome, primary metric, baseline, guardrails, success/stop criteria, owner, budget, evaluation window |
| Product objective | Approved intent, linked hypothesis/opportunity, acceptance criteria, spec revision, dependencies, delivery task IDs |
| Outcome | Released artifact, observed cohort/window, metric change, limitations, decision and evidence links |

Use explicit tenant/product scope on writes, reads, jobs, caches, retrieval indexes and artifacts; enforce authorization in deterministic code. Never depend solely on an agent including a tenant identifier in a prompt. Use current persistence and event primitives rather than introducing a second platform.

## 3. Example configuration (illustrative, not current Nightshift syntax)

```yaml
api_version: proposed/v1
tenant_id: example-tenant
product_id: invoice-helper
strategy_revision: 3
product:
  mission: Help independent consultants prepare invoices faster
  target_segment: Solo consultants who issue recurring invoices
  non_goals: [tax advice, payment processing]
  primary_outcome:
    name: first_valid_invoice_exported
    definition: eligible new user exports a valid invoice within 24h
    denominator: eligible new users in the onboarding cohort
  guardrails: [export_error_rate, privacy_incidents, support_contacts]
product_intelligence:
  mode: propose_only
  discovery_cadence: weekly
  allowed_sources: [approved_feedback, aggregate_product_events, support_labels]
  max_open_opportunities: 10
  budget:
    monthly_research_usd: 30
    max_experiments_per_week: 2
  approval_required: [roadmap_change, new_external_integration, pricing_change,
                      customer_contact, production_release]
  forbidden: [cross_tenant_retrieval, unapproved_personal_data_export,
              changing_metric_definitions_during_evaluation]
```

The numbers and sample product are placeholders. Actual owners decide permitted data, limits, KPIs and release policies. Tenant profile cannot override a platform safety floor; project-specific policy may only narrow or explicitly exercise delegated authority. Snapshot policy and strategy versions for each run. Re-authorize sensitive actions at execution time to respect revocations.

## 4. Role design and decision authority

Start by extending the Lead's context and adding a bounded **product discovery workflow**. Keep Builder responsible for implementation and Verifier responsible for behavioral and policy evidence. Do not automatically add another permanent agent role. A separate Product Strategist/Analyst becomes worthwhile only if independent product judgment, recurring discovery workload or evaluation results justify it. The product reviewer checks hypothesis/evidence; engineering Verifier checks implementation; an authorized owner decides risky business actions.

| Mode | Agent may | Owner retains |
|---|---|---|
| Off | Execute engineering specs only | All product decisions |
| Advisory (default) | Read approved evidence, suggest hypotheses, prioritize draft opportunities | Accept/reject and authorize work |
| Bounded experiments | Start explicitly preauthorized reversible experiments within quotas | Strategy, sensitive actions, budget expansion, risky release |
| Outcome-driven | Propose/execute eligible roadmap slices under scoped policy and monitor outcomes | Strategic direction, legal/privacy, external commitments and promotion limits |

Make mode selectable per product under tenant limits; opt-in and reversible. Deny by default, audit transitions, provide kill switch and manual override. No autonomous pricing, billing, customer messaging or production action unless separately authorized. Do not mistake a proposed strategy for an approved strategy.

## 5. Evidence and discovery

Ingest only authorized sources: structured product events, redacted support tickets, interview summaries, opt-in feedback and operational observations. Prefer aggregated data; avoid copying customer messages and PII into source repositories or prompts unnecessarily. Treat imported text as untrusted data, not tool instructions. Include provenance, sample size, time window, selection bias and contradictory evidence. Distinguish actual observations from hypotheses and synthetic personas. Synthetic users can test UX but cannot validate market demand.

Opportunity triage may use RICE-style reasoning (reach, impact, confidence, effort) or a simple weighted rubric. Start with human-readable assumptions, confidence and estimated cost; never let a numerical score authorize work. Include strategic fit, risk and opportunity cost; low confidence should favor discovery. Deduplicate by product, enforce WIP caps, and close stale hypotheses explicitly.

Example: users abandon invoice creation. Evidence shows recurring confusion at tax selection. Candidate options are copy clarification, a guided flow, or no change while interviewing users. Decide the cheapest falsifiable experiment. Freeze activation definition and guardrails, obtain authorization, ship a reversible change, observe the eligible cohort, and record whether to keep it. If traffic is too low, use interviews and usability sessions and label conclusions directional rather than claiming causal lift.

## 6. Integration plan for Nightshift

Earlier audit described tenant boundaries, Lead/Builder/Verifier, Go dispatcher, PostgreSQL/River, gates, event history, memory, budgets, Slack/CLI/MCP and release paths. Verify these on current HEAD. Reuse existing authorization, task/spec IDs, event ingestion, persistence and UI surfaces where appropriate. Link product-level completion to audit NS-A05; bind engineering evidence to actual artifacts per NS-A01; protect learning/policy boundaries per NS-A02/A04.

- **MVP:** a versioned product profile and owner-approved objective, feedback import from one approved source, weekly advisory proposal with citations, and existing delivery linking. Use a repo-backed profile only for non-sensitive strategy; store sensitive evidence in scoped protected storage.
- **Next:** structured opportunity backlog, outcomes and metric definitions; parent acceptance; feedback ingestion with deduplication; read-only dashboard and bounded schedules.
- **Later:** policy-controlled reversible experiments, rollout/rollback, comparison of cohorts, automated outcome follow-up and roadmap recommendations.
- **Not now:** generic autonomous sales, scraping everything, arbitrary product mutations, complex multi-agent debate, cross-tenant learning by default, or a full product analytics vendor replacement.

Alternative implementation: markdown-only profiles are cheap and portable but awkward for validation and queries; typed DB records offer lifecycle and access control but require schema/UI work; start hybrid (versioned non-sensitive configuration plus typed scoped events/decisions). Pull existing analytics summaries rather than building an analytics warehouse until necessary.

## 7. Acceptance and evaluation

1. Two tenants have distinct goals and evidence. Retrieval, artifacts, events, metric aggregation, and proposals remain scoped; negative tests prove no cross-tenant leaks, including via shared memory and cache.
2. A product configured as Off creates no discovery jobs. Advisory mode cannot create executable work or release without approval. Budget exhaustion and revocation prevent new actions.
3. An opportunity with no real evidence is identified as a hypothesis, not a customer fact. Malicious instructions in feedback do not trigger tool calls or policy changes.
4. One product objective links source evidence, strategy revision, approved spec, integrated acceptance and deployed artifact. A change in strategy or spec invalidates affected pending decisions.
5. An outcome report includes cohort, denominator, time window, baseline, guardrails, confounders and decision. Missing/insufficient evidence produces an inconclusive outcome, not a fabricated win.
6. A simple evaluation compares product-aware Lead versus ordinary spec-only Lead on independently rated opportunities: evidence quality, decision usefulness, scope alignment, wasted work, human review time. Track downstream actual outcomes, not just proposal quality.
7. Tenant opt-out, data deletion and revocation semantics work, including queued work and retained summaries, subject to legitimate audit retention.

## Research leads (not yet verified for this package)

Continuous discovery, opportunity solution trees, Jobs to Be Done, experiment design, product analytics/cohort analysis and outcome-based roadmaps. Investigate original practitioner references and empirical limitations; adopt only techniques needed by tenant products. Product thinking should make Nightshift choose better work, not merely make its specs more verbose.

## Repository reconciliation and retention

This dated external research proposal is not a normative design change. Preserve the snapshot and append links to subsequent decisions and evaluation evidence rather than deleting it when superseded. The core knows no tenant-specific names; implement future capabilities as generic schema/config and tenant packs. This is optional advisory-first research, not an approved product policy or change of authority.

**Checked:** repository rules, status and research index at [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26. External source dates and links are listed in the research pack. **Inferred:** proposed architecture, experiments and maturity targets; no implementation or runtime validation. **Confidence:** medium.
