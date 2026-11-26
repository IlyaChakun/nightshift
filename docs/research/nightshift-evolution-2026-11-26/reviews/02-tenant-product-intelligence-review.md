# Tenant product discovery review (PR #340)

Try one owner-reviewed product brief with evidence the owner has approved for that purpose. Nightshift already turns approved intent into a spec, so the experiment can start with a non-sensitive charter and a manual review. Introduce a product identity and storage only when several products need distinct access rules.

Checked main `8171d5ea85723b73185fd65acb27d4ded7134b82` against research head `d1c8a1d`.

| Claim | Disposition | Evidence at checked main |
| --- | --- | --- |
| Tenant and project schemas define owners, repositories and engineering settings. Product strategy and business outcomes are proposed. | Confirmed. The sample YAML is illustrative. | `schemas/tenant.json:8`, `schemas/project.json:8` |
| Lead intake and source-checked memory can support a brief | Confirmed. Criteria support source quotes. Memory proposals cite owner words or repository files. | `schemas/spec.json:17`, `internal/dispatch/lead.go:374`, `internal/dispatch/leadmemory.go:43` |
| Search and duplicate suggestions have tenant scope and supply no independent demand measure | Confirmed. Search filters by tenant and visibility. A task contributes its first request, which gives no count of distinct customers. Product scope is absent from this query. | `internal/search/search.go:92`, `internal/dispatch/similar.go:63`, `internal/dispatch/similar.go:91` |
| One approved objective has aggregate delivery evidence | Partly. Expansion creates siblings and dependency links, and the Verifier omits the whole-set spec for the expanded task. | `internal/dispatch/deps.go:128`, `internal/dispatch/verify.go:157` |

## Decisions

- **Take — phase 5, pilot only.** Use one versioned owner charter, a manually selected redacted evidence batch, alternatives including no build, and a frozen outcome with a denominator, window and guardrails. Record contradictory evidence. Have an independent reader judge factual support and compare to ordinary Lead advice. Only owner-approved intent enters delivery. Keep restricted feedback out of shared retrieval and authoritative memory.
- **Later — phases 5 and 10, subject to owner agreement.** Add product-scoped identities, typed opportunities/outcomes, a deduplicated import, a bounded schedule, dashboard and approval history if manual briefs improve decisions. Require negative scope tests across two products and two tenants, opt-out, revocation, deletion of derived summaries and cancellation of queued work before ingesting restricted material.
- **Later — phases 6 and 10.** Consider reversible, preauthorized trials after scoped authorization, budget and rollback work in staging. Use qualitative observation for a small cohort. Defer an independent strategist role until review demand justifies it.
- **Reject — all phases.** Automatic customer contact, pricing, spending, unrestricted scraping, cross-tenant learning and an analytics service exceed this pilot's purpose. A prioritization score provides no approval.

## Primary evidence

Read 2026-11-26. Torres recommends “story-based customer interviews”, Intercom uses “reach, impact, confidence and effort”, and Microsoft describes “survivorship bias” in experiments. These sources inform discovery and evaluation. Their studies do not measure this Lead. [Product Talk](https://www.producttalk.org/opportunity-solution-trees/), [Intercom](https://www.intercom.com/blog/rice-simple-prioritization-for-product-managers), [Microsoft Research](https://www.microsoft.com/en-us/research/articles/diagnosing-sample-ratio-mismatch-in-a-b-testing/).

**Checked** Schemas, intake, retrieval, memory and task expansion. **Inferred** Manual trial is sufficient to test advisory value. **Unverified** Customer evidence, an actual product trial, data retention and incremental benefit. **Confidence** High for inspected interfaces, medium for pilot value.
