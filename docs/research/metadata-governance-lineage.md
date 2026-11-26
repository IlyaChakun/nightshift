# Metadata, governance and lineage

**Decision brief, 2026-11-26 — proposed, not adopted.** Nightshift needs trustworthy provenance for engineering evidence and decisions, not a new enterprise data platform. Reuse the book and the current safety work for core provenance; add tenant data lineage only when a real product needs it.

**Research provenance:** this brief is a direct source and repository review in the writing session, not a transfer of the separate Deep Research report, whose completed text is not accessible here. Main is pinned to [8171d5e](https://github.com/IlyaChakun/nightshift/tree/8171d5ea85723b73185fd65acb27d4ded7134b82). No runtime change or design adoption is authorized by this document.

## Contents

- [Separate the responsibilities](#separate-the-responsibilities)
- [Existing evidence and gaps](#existing-evidence-and-gaps)
- [Prior art and alternatives](#prior-art-and-alternatives)
- [Core provenance contract](#core-provenance-contract)
- [Tenant metadata contract](#tenant-metadata-contract)
- [Examples and failure modes](#examples-and-failure-modes)
- [Proposed optional plan stream](#proposed-optional-plan-stream)
- [Evaluation and stop conditions](#evaluation-and-stop-conditions)

## Separate the responsibilities

| Concern | Question | Authority |
| --- | --- | --- |
| Semantic model | What does this entity, metric or rule mean? | Approved domain/schema definition |
| Metadata | Which identity, revision, owner, classification and source describe it? | Versioned source records |
| Data lineage | Which inputs and transformations produced this dataset/metric? | Observations or declared mappings with limitations |
| Software provenance | Which source, build and execution produced this artifact? | Trusted producer and verified digest |
| Trace | What happened during execution? | Instrumentation; may be incomplete |
| Governance | What is permitted, required, retained or disclosed? | Trusted policy and action-time enforcement |

A trace ID, Git SHA, catalog tag or agent-written explanation alone does not establish authorization, behavioral correctness or a complete lineage chain. A policy decision needs exact subject, action, scope, applicable policy and current evidence. A semantic model defines meaning; a metadata catalog is not an enforcement engine. Avoid a second workflow ledger and a second product ontology.

## Existing evidence and gaps

Static inspection of pinned main, not an exhaustive code audit:

| Source | Existing foundation | Candidate gap or limitation |
| --- | --- | --- |
| [tasks.sql](https://github.com/IlyaChakun/nightshift/blob/8171d5ea85723b73185fd65acb27d4ded7134b82/internal/book/queries/tasks.sql#L1-L25) | Tenant/repo task, state/version/head CAS | Explicit objective and criteria provenance belongs with A05, not a new parallel task graph |
| [jobs.sql](https://github.com/IlyaChakun/nightshift/blob/8171d5ea85723b73185fd65acb27d4ded7134b82/internal/book/queries/jobs.sql#L20-L76) | Role/provider job, claim attempt, lease fence, final usage/result and artifact hashes | Reclaimed job row is reused; physical-attempt history and artifact-to-attempt binding need verification. Artifact insert shown keys by job, not attempt |
| [events.sql](https://github.com/IlyaChakun/nightshift/blob/8171d5ea85723b73185fd65acb27d4ded7134b82/internal/book/queries/events.sql#L1-L17) | Actor, payload, trace ID, decision kind/SHA/card version/reason | Preserve existing decisions; verify complete spec/policy/actor/action evidence at final gate rather than treating a decision row as universal authorization |
| [Mover](https://github.com/IlyaChakun/nightshift/blob/8171d5ea85723b73185fd65acb27d4ded7134b82/internal/flow/move.go#L42-L78) | State update and transition event in one transaction | Reuse atomic transition; audit surrounding result consumption/outbox under A03 |
| [completion API](https://github.com/IlyaChakun/nightshift/blob/8171d5ea85723b73185fd65acb27d4ded7134b82/internal/jobsapi/server.go#L340-L421) | Lease-authenticated completion transaction, output records | Worker-origin data is not independent proof of acceptance; confirm trusted artifact/evaluator identities |
| [Verifier enqueue](https://github.com/IlyaChakun/nightshift/blob/8171d5ea85723b73185fd65acb27d4ded7134b82/internal/dispatch/verify.go#L53-L96) | Base/head review inputs | Exact integrated candidate, spec and required-suite identity must be checked through release boundary, not merely carried as prompt context |
| [task tracing](https://github.com/IlyaChakun/nightshift/blob/8171d5ea85723b73185fd65acb27d4ded7134b82/internal/dispatch/tracing.go#L14-L25) | Correlation across jobs | Trace correlation does not prove complete or immutable evidence |
| [knowledge search](https://github.com/IlyaChakun/nightshift/blob/8171d5ea85723b73185fd65acb27d4ded7134b82/internal/search/search.go#L90-L131), [Lead memory](https://github.com/IlyaChakun/nightshift/blob/8171d5ea85723b73185fd65acb27d4ded7134b82/internal/dispatch/leadmemory.go#L37-L93) | Tenant visibility, source/date, status and quote validation | Repository source references need revision context where historical reproducibility matters; no product ID in inspected search filter |

Prior research: [#332](https://github.com/IlyaChakun/nightshift/pull/332) release evidence, [#334](https://github.com/IlyaChakun/nightshift/pull/334) durability, [#335](https://github.com/IlyaChakun/nightshift/pull/335) isolation, [#336](https://github.com/IlyaChakun/nightshift/pull/336) objective acceptance, [#345](https://github.com/IlyaChakun/nightshift/pull/345) attempt economics, [#348](https://github.com/IlyaChakun/nightshift/pull/348) decision binding, [#350](https://github.com/IlyaChakun/nightshift/pull/350) experiment provenance and [#351](https://github.com/IlyaChakun/nightshift/pull/351) knowledge promotion. These are research references, not assumed merged implementations. Reconcile their invariants rather than create separate fixes for the same missing identities.

## Prior art and alternatives

Primary documentation inspected 2026-11-26. Claims describe documented interfaces, not measured Nightshift performance.

| Source | Relevant technique | Limit |
| --- | --- | --- |
| [W3C PROV-DM](https://www.w3.org/TR/prov-dm/), Recommendation 2013-04-30 | Entity/activity/agent and derivation/attribution relations | Conceptual vocabulary; no requirement for RDF, graph database or automatic trust |
| [OpenLineage object model](https://openlineage.io/docs/spec/object-model/) (page version 1.52.0) | Job/run/dataset separation, runtime observations vs design metadata, extensible facets | Data pipeline events; not release authorization or proof of complete coverage |
| [SLSA v1.2 provenance](https://slsa.dev/spec/v1.2/provenance) | Verifiable build/source provenance and artifact identity | Provenance does not prove behavior or grant approval; evaluate signing only when needed |
| [OpenTelemetry traces](https://opentelemetry.io/docs/concepts/signals/traces/) | Span correlations, attributes and links | Operational telemetry may be sampled; never make it the authoritative required evidence store |
| [DataHub lineage](https://docs.datahub.com/docs/features/feature-guides/lineage) | Lineage discovery and impact-analysis integration surface | Vendor documentation; operational fit, licensing and coverage not tested |
| [OpenMetadata documentation](https://docs.open-metadata.org/latest) | Catalog, contracts, quality, lineage, governance, connectors | Broader data platform; deployment, permissions and actual integration costs unverified |
| [OPA documentation](https://www.openpolicyagent.org/docs) | Separate policy decision from enforcement | Do not add another policy engine without a demonstrated gap; a caller must enforce the verdict |
| [dbt manifest](https://docs.getdbt.com/reference/artifacts/manifest-json) | Versioned project metadata artifact; input for design-time lineage | Not evidence of an actual transformation run; use matching runtime evidence where required |

Research leads, not inspected in this pass: Apache Atlas, in-toto's full attestation specification, OpenLineage column facets/connectors, schema registry compatibility and contract standards, catalog licensing/deployment sizing. No price, completeness or compliance claims.

| Option | Cost and benefit | Decision |
| --- | --- | --- |
| Existing book joins, source references and explicit evidence fields | Lowest integration overhead; limited cross-system discovery | Start here |
| Small typed relation projection in Postgres | Bounded graph traversal and impact queries; needs update/rebuild/authorization semantics | Add only for observed missing queries |
| Read-only import from a tenant's existing catalog or dbt artifacts | Reuse real domain metadata; source availability, staleness and permissions matter | Optional tenant capability |
| Adopt full catalog platform or separate graph DB | Rich connector/UI ecosystem; extra services, trust boundary, upgrades and support | Defer until a tenant proves need; compare build vs existing tool |
| Universal shared metadata/lineage service across tenants | Broad discovery but sensitive identifiers, poisoning and disclosure risks | Reject implicit sharing |

## Core provenance contract

Treat existing canonical records as nodes. Derive links already implied by foreign keys and trusted state transitions; add only relationships that cannot be recovered reliably. Candidate concepts below are not implemented schema fields.

| Relationship | Evidence and identity |
| --- | --- |
| Objective revision contains requirement; requirement allocated to task | Owner-approved spec digest and explicit membership (A05) |
| Job has physical attempt | Trusted claim/lease event, attempt ID and final disposition; retain failed and expired attempts |
| Attempt uses input and generates artifact | Source/base/tree, effective role/model/skills/toolchain, artifact digest; distinguish declared vs observed configuration |
| Independent check evaluates candidate | Actual tested artifact, suite and evaluator version, policy/spec revision, environment, outcome and completeness |
| Decision authorizes action | Trusted actor/scope, expected task/spec version, exact candidate, required evidence, current policy |
| Release deploys artifact | Target/environment, authorized action, actual deployed digest, observed recovery or failure |
| Finding/incident motivates improvement | Observed occurrence, cause hypothesis, linked experiment, independently approved promotion/revocation |

A required edge is written at its authoritative transaction boundary, not reconstructed from a natural-language summary. Reuse A01/A03 for atomic decision and acceptance records. The relation projection is rebuildable and advisory, not an authorization source. Missing mandatory evidence at the action boundary blocks the action. Missing advisory lineage is visibly unknown, not proof of no dependency.

Represent relation identity with scoped endpoints, relation type, source record/version, assertion kind (declared, observed, inferred), producer identity, observation time, validity interval/revision and classification. Preserve contradictory observations rather than last-write-wins; adjudicate using the relevant source of truth. Deduplicate by stable source event/edge identity, not text. Use bounded traversal with visited identities; feedback loops and versioned derivations can form cycles at logical-object level. For immutable artifacts, distinguish generation activity and version from a mutable name.

No new event sourcing system. If a read model is useful, benchmark indexed joins or recursive SQL over tenant-scoped relations before evaluating separate graph storage. Keep trusted writes transactionally coupled to the existing book, and make any optional projector replayable. Never let a late projector overwrite authoritative decision or release state.

## Tenant metadata contract

Off by default; tenant owner selects an actual product need and permitted sources. A non-sensitive versioned glossary and data contract in the repository can be sufficient. Do not collect a company's data estate just because a coding agent is available.

Potential scoped resources: logical business entity, physical dataset/field, API/event/schema revision, producer/consumer, transform, metric definition, owner/steward, classification, retention rule and source system. Keep logical meaning distinct from physical storage and operation history. Multiple products in a tenant may need separate visibility boundaries; do not assume repo equals product.

Begin with one source, such as a pinned schema or approved dbt artifact. Import metadata, not customer rows or raw query results. Record source coverage, permission, schema/version, observed time and expiry; distinguish static inference from observed runtime lineage. A metric records denominator, unit, time window, aggregation, exclusions and transformation dependencies. An invariant links its approved definition to independently executed tests; the declaration itself is not enforcement.

Lead uses scoped context to clarify requirements and potential impact. Builder gets public contract and permitted implementation links. Verifier gets exact approved requirements and independent test inputs according to policy. Imported descriptions, SQL comments and tool responses are untrusted data. A model may propose a mapping; only a trusted source or authorized review establishes it.

Governance: enforce scope at query, traversal, artifact, cache and export boundaries. Do not reveal even existence/count of forbidden nodes or intermediate paths. Dataset names, query text, lineage and owner details may be confidential. Redact and minimize before logs/export; no secrets in identifiers, no raw private evidence in public Git history. Retention/deletion and legal audit obligations need owner-approved policy; immutable identity does not mean unlimited retention. Track supersession, revocation and derived projections; do not promise recall of already exported copies.

## Examples and failure modes

**Synthetic factory release:** objective O1 revision 2 requires retry-safe operation. Task T1 has failed attempt A1 and successful A2. A2 produces artifact digest H1. An independent test passes H1 under spec revision 2 and policy P1. Owner approval authorizes H1 for staging only. If a rebase produces H2, the old evidence is not automatically valid; re-evaluate under current policy. A production release requires its own applicable authorization. A later incident links to the actual deployed version, not merely the last merged PR. Preserve A1 cost and diagnostic evidence. This is a proposed scenario, not a live test result.

**Synthetic payments analytics tenant:** an approved contract changes capture amount from one representation to another. Design lineage links the API event field to a transformation and a daily captured-amount metric; runtime observations may add actual consumer runs. Lead identifies known affected consumers, states unknown ones, and requests contract/metric checks. Builder migrates compatible producers/consumers. Verifier checks unit semantics, duplicate events and unchanged metric definition against fixture data. No assumption about a real payment provider or customer's production schema.

Required negative scenarios:

- Wrong candidate, changed spec or revoked approval cannot pass a mandatory release check.
- Reclaimed lease, duplicated/out-of-order event, lost acknowledgement and projector crash do not corrupt attempt history or action authority.
- Stale source, missing column mapping, unobserved consumer, conflicting owner classification and failed connector remain explicit unknown/conflict states.
- Two tenants/products with overlapping names cannot traverse each other's restricted nodes, caches, summaries or derived edges.
- Malicious catalog description or fabricated worker evidence cannot create privileges or satisfy independent acceptance.
- Retention/revocation removes access to derived data according to approved rules; historical redacted audit record retains only permitted facts.

## Proposed optional plan stream

Current roadmap uses `docs/plan.md` and owned files under `docs/plan/`. Do not edit another phase's file. This is an exact proposed addition for owner/coordinator acceptance, not an active phase or an adopted design. Keep safety fixes in their original phase/work package; do not defer required provenance until this optional stream.

Proposed new stream row in the owner-requested streams table:

    | Semantic context, metadata and lineage | proposed, deferred | [plan/semantic-context.md](plan/semantic-context.md) |

Proposed file `docs/plan/semantic-context.md` (not created by this research PR):

    # Semantic context, metadata and lineage

    Status: proposed, not started; no change to current phase order.
    Dependencies: core release/durability/isolation proof, one real pilot,
    accepted semantic architecture and evidence contracts where applicable.

    - [ ] ML1. Core provenance inventory: map existing identities and links;
      merge required evidence gaps into existing safety work, not a new ledger.
    - [ ] ML2. Read-only lineage pilot: one scoped objective-to-release query
      with explicit unknowns; prove replay and exact-candidate correctness.
    - [ ] ML3. Optional tenant metadata: one approved schema/contract source,
      product/tenant scope and one real impact-analysis use case.
    - [ ] ML4. Compare baseline and structured context; decide whether richer
      catalog integration is warranted. Retain negative results.

    Done: one independently checked core provenance query; one opted-in tenant
    use case with measured impact and negative scope/staleness tests, or
    documented evidence-based deferral. No graph DB or catalog service required.

If adopted architecture changes, write the appropriate ADR and synchronize design Markdown/HTML as repository rules require. Coordinate with the separate Semantic Architecture research so the domain vocabulary and metadata contract remain one shared proposal, not duplicate schema systems.

## Evaluation and stop conditions

First inventory mandatory links and the queries an operator actually cannot answer. Demonstrate one missing link with a focused repository test; correct authoritative records where needed. Then compare existing book/search context against minimal structured lineage on frozen synthetic tasks and a scoped real pilot. Only add a third catalog/graph arm if first two leave a material unresolved need. Independent reviewers label known relationships and affected requirements; include negative, stale and incomplete cases.

Measure verified required-link coverage with explicit denominator, observed vs declared/inferred edges, precision/recall against a labeled impact set, stale evidence rejection, unknown rate, missed affected components, query/ingestion overhead, active operator minutes and cost per independently accepted objective. Count missing/failed runs; never reward fabricated coverage or suppress unknowns. Thresholds and allowed cost are owner-approved before trials.

Mandatory safety proof: no unauthorized access, no fabricated required evidence, no stale-artifact authorization in the specified negative tests. Optional utility gate: measurable reduction in missed impact or operator effort without quality or resource regression. If joins and approved domain documents suffice, stop there. If catalog source access, retention or trust boundaries are not justified, keep the tenant capability disabled.

**Checked:** current main, selected SQL/Go paths, plan/status and repository rules, primary documentation listed above. **Inferred:** candidate lineage contracts, risks and sequencing. **Confidence:** high in reuse-first boundary; medium in optional tenant benefit. **Not verified:** separate Deep Research report contents, full current-code inventory, live integrations, catalog cost/licensing, actual lineage completeness, integration tests, CI and local make verify.
