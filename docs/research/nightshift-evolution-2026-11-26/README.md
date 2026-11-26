# Nightshift — research, vision and real-world validation

**Date:** 2026-11-26. **Status:** external research and proposals for review; documentation only.

| Document | Purpose |
|---|---|
| [01 — Industry case studies](01-industry-case-studies.md) | Six industrial examples, reported evidence, limitations, transferable techniques and targeted experiments |
| [02 — Tenant product intelligence](02-tenant-product-intelligence.md) | Product strategy, scoped evidence, hypothesis/experiment loop, tenant policies and minimal implementation |
| [03 — Autonomy vision](03-autonomy-vision.md) | Five maturity levels, Nightshift mapping, dual learning loops and evidence-based exit gates |
| [04 — Product pilot and measurement](04-product-pilot-and-measurement.md) | Real product launch, baseline comparison, monitoring, metrics, intervention log and promotion criteria |

## Agent instructions

1. Read existing `AGENTS.md`, architecture, current status, research index, ADRs and prior audit (NS-A01–A07 if imported). Pin HEAD. Reconcile each claim and proposal with the current implementation. The external audit was selective; current GitHub fetch failed. Do not assume a missing capability from these documents.
2. Integrate research into the existing knowledge base with sources and evidence status. Do not duplicate existing docs or backlog. Keep historical snapshots distinct from current status.
3. Produce a capability/gap matrix with confirmed evidence, owner, priority and simplest useful experiment. Prefer fixing safety and running a real product pilot over large architectural changes.
4. Define a product charter with the tenant owner. Begin with advisory product intelligence. Do not start production actions, collect personal data, contact customers, grant privileges or increase budgets without authority.
5. Establish a baseline and independent evaluation before changing the factory. Record failures, resource cost and human time; evaluate changes on holdouts, and keep rollback and promotion external to the candidate.
6. Implement only justified increments, with relevant tests and documentation. Report actual results, limitations and remaining risks. Escalate ambiguous business decisions to the tenant owner.

## Additional questions that must not be overlooked

- **Evaluation ownership:** who controls hidden scenarios, benchmark splits, metric definitions and artifact-bound evidence? Can a candidate alter its own pass criteria?
- **Economics:** is orchestration worth the overhead relative to a standalone agent? Measure total engineering and operating cost, not tokens alone; define per-tenant budgets and chargeback before commercial use.
- **Product-market validation:** which customers have a real problem, what alternatives exist, what do they actually use or pay for? Passing acceptance tests is not demand. Keep product learning separate from factory learning.
- **Human attention:** record time spent specifying, approving, reviewing, rescuing and maintaining the factory; never count hidden human work as autonomous execution.
- **Tenant trust:** protect credentials, customer data, evaluation artifacts and memory; explicitly govern any cross-tenant knowledge sharing. Define retention, deletion, access revocation and audit requirements. Treat external feedback as untrusted input.
- **Operations:** define ownership and incident response, backups, restore drills, kill switches, reversible release and rollback. An unattended system still has an accountable operator.
- **Supply chain and licensing:** dependency provenance, generated-code license risks, secret scanning and repository/CI trust. Review platform and provider terms before public/commercial deployment.
- **Long-term maintainability:** observe repeated changes to a real product, architectural drift and accumulated technical debt; maintain independent regression tests and periodically evaluate behavior on new tasks.

Suggested order: validate current safety and durability → establish a baseline → launch one useful product → add minimal tenant product context and integrated acceptance based on actual need → collect operational and product signals → run controlled factory improvements → expand autonomy only on evidence. The maturity model is not a mandate to build every feature in advance.

## Retention and disposition

Keep this dated owner-requested external review as a historical source, not an alternative living design. Preserve the original findings and append links to decisions, implementation PRs, regression tests and current disposition. Do not remove or silently rewrite the record during plan/status cleanup; request explicit owner approval before deletion or relocation. This is a documentation convention, not a technical permission boundary. The canonical design remains `docs/design.md`; reconcile durable research with `docs/prior-art.md` and `docs/research/README.md`.

**Checked:** repository documentation rules at [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26. **Inferred:** retention convention and sequencing. **Confidence:** high for the intended organization; no local checks performed.

## Complete research inventory and consolidation (2026-11-26)

The table below expands the initial four-document index. See [audit consolidation](../../audit/2026-11-26/README.md) for evidence vocabulary, shared invariants, implementation dependencies and negative tests. Do not interpret each proposal as an implementation ticket.

| Document | PR | Disposition |
| --- | --- | --- |
| [Industry case studies](01-industry-case-studies.md) | [#339](https://github.com/IlyaChakun/nightshift/pull/339) | Research context; calibrate claims and extract experiments |
| [Tenant product intelligence](02-tenant-product-intelligence.md) | [#340](https://github.com/IlyaChakun/nightshift/pull/340) | Owner-approved advisory discovery |
| [Autonomy vision](03-autonomy-vision.md) | [#341](https://github.com/IlyaChakun/nightshift/pull/341) | Evidence-gated responsibility and authority |
| [Product pilot and measurement](04-product-pilot-and-measurement.md) | [#342](https://github.com/IlyaChakun/nightshift/pull/342) | Frozen cohort and standalone baseline |
| [Codebase intelligence](05-codebase-intelligence.md) | [#344](https://github.com/IlyaChakun/nightshift/pull/344) | Advisory native impact brief |
| [Engineering economics](06-engineering-economics.md) | [#345](https://github.com/IlyaChakun/nightshift/pull/345) | All-attempt usage and outcome cost |
| [Production intelligence](07-production-intelligence.md) | [#346](https://github.com/IlyaChakun/nightshift/pull/346) | Incident-to-independent recovery |
| [Parallel engineering](08-parallel-engineering.md) | [#347](https://github.com/IlyaChakun/nightshift/pull/347) | Current integrated candidate fencing |
| [Human attention](09-human-attention.md) | [#348](https://github.com/IlyaChakun/nightshift/pull/348) | Version-bound decision queue |
| [Technical debt](10-technical-debt.md) | [#349](https://github.com/IlyaChakun/nightshift/pull/349) | Measured maintenance cost and invariants |
| [Factory experimentation](11-factory-experimentation.md) | [#350](https://github.com/IlyaChakun/nightshift/pull/350) | Protected evaluation and trial history |
| [Cross-tenant learning](12-cross-tenant-learning.md) | [#351](https://github.com/IlyaChakun/nightshift/pull/351) | Reviewed synthetic promotion only |
| [Capability architecture](13-capability-architecture.md) | [#352](https://github.com/IlyaChakun/nightshift/pull/352) | Presets before manifests; defer runtime |

### Verified GitHub snapshot

The connector returned main [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15) and all 21 PRs as open drafts, unmerged, after transferring the final Deep Research sections to #351, #352 and #341. The report's public-page repository-state impression was inconsistent with the connector; the exact connector snapshot is used here. #343's own head below is the pre-edit anchor, not the final self-referential commit.

| PR | State | Verified head | Evidence package |
| --- | --- | --- | --- |
| [#332](https://github.com/IlyaChakun/nightshift/pull/332) | Open draft | [cf6fd6a7cee6](https://github.com/IlyaChakun/nightshift/commit/cf6fd6a7cee60e69b91146ac63df245b2983be80) | Earlier decision brief |
| [#333](https://github.com/IlyaChakun/nightshift/pull/333) | Open draft | [cbfded084278](https://github.com/IlyaChakun/nightshift/commit/cbfded0842783e970c7bef26c7e43f80c7912a4a) | Earlier decision brief |
| [#334](https://github.com/IlyaChakun/nightshift/pull/334) | Open draft | [ae10f2857e4d](https://github.com/IlyaChakun/nightshift/commit/ae10f2857e4d03df57b080d4366f733d3da48481) | Earlier decision brief |
| [#335](https://github.com/IlyaChakun/nightshift/pull/335) | Open draft | [86892fce2410](https://github.com/IlyaChakun/nightshift/commit/86892fce24106a152f9d0e867462a1a521338cbf) | Earlier decision brief |
| [#336](https://github.com/IlyaChakun/nightshift/pull/336) | Open draft | [4e7d31982f9a](https://github.com/IlyaChakun/nightshift/commit/4e7d31982f9acc45fcd0ce018dd708a9fdaa970c) | Earlier decision brief |
| [#337](https://github.com/IlyaChakun/nightshift/pull/337) | Open draft | [f6ad570b70a6](https://github.com/IlyaChakun/nightshift/commit/f6ad570b70a64c8e2fb7d2b934f126bad98b3eb4) | Earlier decision brief |
| [#338](https://github.com/IlyaChakun/nightshift/pull/338) | Open draft | [76326927acde](https://github.com/IlyaChakun/nightshift/commit/76326927acde3333e15a81fbad1f9176b46b0b92) | Earlier decision brief |
| [#339](https://github.com/IlyaChakun/nightshift/pull/339) | Open draft | [f743e4320bd2](https://github.com/IlyaChakun/nightshift/commit/f743e4320bd20ef2fd0d8530d15fb3ef7f7308f4) | Earlier decision brief |
| [#340](https://github.com/IlyaChakun/nightshift/pull/340) | Open draft | [d1c8a1dfebdc](https://github.com/IlyaChakun/nightshift/commit/d1c8a1dfebdcf42eadfd362e4e3ebe92688b5cc5) | Earlier decision brief |
| [#341](https://github.com/IlyaChakun/nightshift/pull/341) | Open draft | [6b670075040a](https://github.com/IlyaChakun/nightshift/commit/6b670075040a33fe1021f9b5f81b02ac25d0c7e7) | Completed Deep Research transfer |
| [#342](https://github.com/IlyaChakun/nightshift/pull/342) | Open draft | [ba157272f31f](https://github.com/IlyaChakun/nightshift/commit/ba157272f31f56354acfe4875975d974f36cc714) | Earlier decision brief |
| [#343](https://github.com/IlyaChakun/nightshift/pull/343) | Open draft | [e8e2f943c1e5](https://github.com/IlyaChakun/nightshift/commit/e8e2f943c1e517622bdc46c87e2ba33b73bcf060) | Pre-consolidation head |
| [#344](https://github.com/IlyaChakun/nightshift/pull/344) | Open draft | [5ab0f47321ad](https://github.com/IlyaChakun/nightshift/commit/5ab0f47321ad22034ba1d30640a19b95a97b4289) | Earlier decision brief |
| [#345](https://github.com/IlyaChakun/nightshift/pull/345) | Open draft | [2edf7b62dd00](https://github.com/IlyaChakun/nightshift/commit/2edf7b62dd00f09c7faba61687f4fa8ad0707772) | Earlier decision brief |
| [#346](https://github.com/IlyaChakun/nightshift/pull/346) | Open draft | [03d9f6281d7a](https://github.com/IlyaChakun/nightshift/commit/03d9f6281d7ac0a5bd14200d33cb8166279c1825) | Earlier decision brief |
| [#347](https://github.com/IlyaChakun/nightshift/pull/347) | Open draft | [db0ed1f175e3](https://github.com/IlyaChakun/nightshift/commit/db0ed1f175e339577c7197939f137ad659acaf39) | Earlier decision brief |
| [#348](https://github.com/IlyaChakun/nightshift/pull/348) | Open draft | [796f2bcd7e66](https://github.com/IlyaChakun/nightshift/commit/796f2bcd7e66541c9bd0895b97b8c712246fe916) | Earlier decision brief |
| [#349](https://github.com/IlyaChakun/nightshift/pull/349) | Open draft | [519df247dca6](https://github.com/IlyaChakun/nightshift/commit/519df247dca6d99c183a8cad5f82ebac25f980fd) | Earlier decision brief |
| [#350](https://github.com/IlyaChakun/nightshift/pull/350) | Open draft | [16364c4424f0](https://github.com/IlyaChakun/nightshift/commit/16364c4424f0d6fd3ce7de48f237d77845141dd5) | Earlier decision brief |
| [#351](https://github.com/IlyaChakun/nightshift/pull/351) | Open draft | [2ccc3c91efb7](https://github.com/IlyaChakun/nightshift/commit/2ccc3c91efb721ad72c41efd35955d7966fa15bd) | Completed Deep Research transfer |
| [#352](https://github.com/IlyaChakun/nightshift/pull/352) | Open draft | [eea3d489a3b1](https://github.com/IlyaChakun/nightshift/commit/eea3d489a3b109a7d5ec75565bd16a37935afce0) | Completed Deep Research transfer |

The 20 topic PRs have expanded documents; the index is the 21st PR. Earlier briefs are not all completed Deep Research reports: #339/#342 contain separately authored decision supplements; original prior Deep Research report transfer was not independently confirmed. Treat their provenance and any external numerical claims as requiring source reconciliation. The final Deep Research report is available in this session and its #351/#352 briefs and #341 validation are explicitly transferred, with edits and source URLs retained. #343 is a consolidation based on that report and the live connector inventory, not a new empirical study.

### Canonical prior-art reconciliation

Topic PRs that add shared index entries will conflict if applied blindly from the same base. Reconcile each with main in its own review thread; preserve previous entries. Earlier documents #332–335, #338–339 and #342 have not all added dedicated canonical entries. The final reviewer should link their relevant existing research entries or add missing source references with actual evidence status, not duplicate the same source claims. Check all links and TOCs, maintain historical dates and explicit unverified labels. Do not mark a source read solely because its URL is listed. Verify prior-art entries against the actual merged files before merging this index.

### Review sequence

Safety and durability (#332–334), execution and operational proof (#335/#338), approval and integrated candidate integrity (#348/#347), product acceptance and convergence (#336/#337), measurement baseline (#342/#345) with advisory efficiency (#344), product/production feedback (#340/#346), measured debt and experimentation (#349/#350), optional sharing (#351) and capability composition (#352). #339 provides context throughout, #341 is strategic synthesis; #343 merges last. Baseline measurement can start early without widening authority. This is proposed dependency ordering for implementation, not approval to merge or change code.

**Checked:** all 21 PR head/state entries from GitHub connector in this transfer session; completed final report; existing index. **Not verified:** local make verify, CI, merged-state link resolution, fresh full code audit, empirical improvement or independent re-opening of report sources.
