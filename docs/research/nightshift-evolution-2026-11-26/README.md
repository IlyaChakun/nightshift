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
