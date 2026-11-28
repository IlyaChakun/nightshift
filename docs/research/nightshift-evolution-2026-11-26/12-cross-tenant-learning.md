# Cross-tenant learning and knowledge promotion

Reuse validated generic engineering lessons across eligible tenants without transferring their private context. Begin with an opt-in, human-reviewed promotion path for synthetic rules and tests; personal and work trust domains remain separate.

**Priority:** P2. **Status:** research proposal, not approved implementation.

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
- [Deep research decision brief — privacy-preserving promotion](#deep-research-decision-brief--privacy-preserving-promotion)

## Problem

A factory that learns only within a project repeatedly pays for common mistakes. A shared memory index can leak code, customer information, repository identifiers, business logic or security details. Even a generalized lesson can retain confidential facts, or introduce a rule that is valid for one tenant but harmful to another.

## How it works

Keep private findings, transcripts, source artifacts and project memory in their tenant and instance. Propose a portable knowledge artifact from an eligible finding: a generic rule, skill, synthetic regression case or stack-pack improvement. Include category, scope, source eligibility, derivation, test evidence, confidence, version and a review decision. Use synthetic examples with no dependence on the original repository.

Apply a separate promotion gate for confidentiality, license and permission, generality, correctness and compatibility. Only approved artifacts enter the shared pack or standards repository. Receiving tenants opt into applicable versions; inherited rules cannot weaken their local policies. A rejected or revoked promotion remains traceable without publishing private source material.

## Trade-offs

| Option | Benefit | Limitation |
| --- | --- | --- |
| Tenant-local learning only | Strong isolation, simple governance | Repeated work |
| Reviewed synthetic knowledge artifacts | Reuse with bounded disclosure | Review effort and imperfect sanitization |
| Shared retrieval over raw tenant data | High contextual richness | Violates trust boundaries; reject |
| Automated cross-tenant promotion | Faster diffusion | Leakage and poisoning risk; defer |

Start with explicit manual promotion inside a permitted trust domain. Default-deny sharing, including between personal and work instances. Do not add a global vector search over tenant data. If safe generalization cannot be demonstrated, keep the lesson local.

## Sources

- Design sections 9, 10, 13 and 16: existing memory types, learning provenance, standards precedence and data boundaries.
- [NIST SSDF 1.1](https://csrc.nist.gov/pubs/sp/800/218/final), search excerpt checked 2026-11-26: secure development practices are a reference for review and provenance, not a privacy certification.
- Prior research PRs #333, #335 and #341 cover independent trust, isolation and self-improvement.

## Fit

Nightshift already distinguishes project memory from shared standards and has personal/work instances, tenant overlays and a fix ladder. Reuse the PR-based rule and skill lifecycle. The core must not contain real tenant or repository names, and no private evidence should appear in public GitHub history.

## Status

Partial foundation: the tenant-aware design, memory, standards and learning components are documented; live multi-tenant isolation remains an open check. A governed promotion flow across eligible tenants is a research proposal, not an established capability.

## Proposal

Choose a synthetic, non-confidential failure, such as an unsafe concurrent update. Produce a minimal independent reproducer and generic check; review for leakage, rights, applicability and correctness. Test against the source-shaped synthetic failure, clean examples and a second synthetic tenant. Publish only the approved artifact through a versioned pack PR, initially advisory.

Track source and receiving scopes in the private audit record; the public artifact contains no source tenant data. Measure false positives, recurrence reduction, adoption, operator review effort and ability to revoke. Use explicit revocation metadata and a rollback path; a bad shared rule must not propagate indefinitely.

## Acceptance and stop conditions

- Cross-tenant search and unauthorized artifact reads fail; raw findings, prompts and customer data are never shared.
- A planted sensitive marker or tenant identifier prevents promotion.
- A generic lesson passes independent synthetic evaluation and can be disabled or rolled back under existing policy.
- A receiving tenant's stricter constraints always prevail.
- Different trust domains cannot exchange lessons implicitly, even when operated by one person.
- Defer automation until live isolation, consent and review are proven; retain local learning if review cost outweighs reuse.

## Agent follow-up

Complete the tenant isolation work and reconcile PRs #333, #335, #341 and the factory experimentation proposal. Establish the information-flow policy before any shared retrieval or automatic promotion.

## Record and evidence

Preserve this dated owner-requested proposal. Link later experiments, decisions and implementation PRs; do not silently delete or replace the source record. The living design remains `docs/design.md` and the canonical sources index is `docs/prior-art.md`.

**Checked:** [base 9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; design sections 9, 10, 13, 14, 16, 17, 20, plan, status, research index and open PR titles. **Inferred:** candidate gaps, priorities and experiment design. **Confidence:** medium; no live validation or full code audit. External links identified above were checked only to the extent stated.

## Deep research decision brief — privacy-preserving promotion

**Date:** 2026-11-26. **Disposition:** P2; defer implementation until isolation and independent evaluation are demonstrated.

**Provenance:** transferred and edited from the completed Deep Research report, "Nightshift deep research: final block", 2026-11-26. The research session had no GitHub write access. The subsequent GitHub connector check found main at 9473fa4bc294825c110b6ec236da7110d0ff0e15, not a newer confirmed revision; all 21 research/audit PRs were open drafts. This supersedes the report's unpinned public-page impression of main advancing and six open PRs. External references below are from the report's primary-source excerpts, not a fresh full-text verification by the committing session.

**Decision:** do not build shared retrieval, shared embeddings, centralized transcript analysis, or automatic cross-instance learning. Preserve the instance boundary. The first acceptable mechanism is explicit promotion of an independently reproducible, tenant-neutral artifact whose usefulness can be demonstrated without access to the source tenant.

### Evidence and research basis

| Claim | Classification |
| --- | --- |
| An instance is Nightshift's trust boundary; multiple tenants can share resources inside one trust domain | Documented design, not proof of runtime isolation |
| Maintainer reporting is off by default and restricted to counts unless separately authorized | Accepted ADR 0016; implementation and live enforcement not established here |
| Live two-tenant isolation is complete | Not verified; check A04/A07 and current status |
| Synthetic artifacts are inherently anonymous | Rejected assumption; privacy research identifies residual disclosure risk |
| Promotion improves Nightshift outcomes | Proposal; no measured experiment |

Repository grounding: [design](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/design.md), [ADR 0016](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/adr/0016-improvement-reports-per-tenant-and-per-operator.md), [status](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/status.md), [plan](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/plan.md), [tenant-scoped search](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/search/search.go), [memory provenance](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/leadmemory.go), [learning clusters](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/learn/cluster.go). Code links anchor the relevant existing components; the Deep Research session did not perform a complete current-code audit.

- [Anthropic Clio, 2024-12-11](https://www.anthropic.com/research/clio) uses abstraction, aggregation and privacy validation to expose aggregate usage patterns rather than raw conversations to analysts. Its layered safeguards motivate a separate disclosure review; this is not a proof of anonymity or an argument for centralized transcript collection.
- [NIST SP 800-188, 2023](https://csrc.nist.gov/pubs/sp/800/188/final) distinguishes data-sharing models and recommends governance and assessment of disclosure/re-identification risk. [NIST IR 8053, 2015](https://www.nist.gov/publications/de-identification-personal-information) surveys residual re-identification risk.
- [Membership inference against synthetic health data, 2022](https://www.sciencedirect.com/science/article/pii/S1532046421003063) provides a warning that synthetic generation can retain information about source membership. This is a different setting from hand-authored software fixtures; it does not quantify Nightshift leakage.
- [NIST AI 600-1, 2024](https://www.nist.gov/publications/artificial-intelligence-risk-management-framework-generative-artificial-intelligence) supplies broader risk-management context, not certification.

### Promotion contract

The promoted object should be one of: deterministic check, synthetic regression test, generic skill fragment, generic template or generic stack/pack rule. It should not contain a transcript excerpt, source patch, source embedding, task summary, repository identifier, customer terminology, proprietary constant or generated representation retaining those values.

Separate private lineage from portable artifact. Private lineage records source tenant, finding IDs, approvals and derivation inside the source trust domain. The portable artifact contains only generic identifier/version, applicability contract, license/rights decision, synthetic fixtures, evaluation evidence, non-identifying process provenance, compatibility constraints and revocation state. Export requires specific source authorization and receiving scope opt-in; do not infer consent from shared operators or broad tenant settings.

A sanitization model is not an authority. Reject promotion when planted canaries, repository-specific identifiers, secret-like tokens, private paths, customer nouns, source-domain URLs or distinctive literals survive transformation. These checks detect obvious leakage, not all semantic or membership leakage. An independent disclosure reviewer must assess the artifact without exposing source-private data outside the authorized domain. If that assessment is impossible, keep the lesson local.

Poisoning boundary: a source tenant must not write directly into another tenant's active rules. Promotion produces a candidate; an independent evaluator runs synthetic positive cases, negative cases and unrelated clean cases. Receiving scope opts into an immutable version. Local policy and protected floors always dominate imported policy.

Revocation: stable artifact ID and immutable version; reject new adoption of revoked versions, mark derived active configurations stale, and preserve derivation edges for downstream review. Do not silently delete historical evidence; privacy/retention obligations still apply. Revocation does not magically recall already exported copies: maintain an explicit notification/rollback path and record residual exposure.

Licensing: generalized knowledge is subject to reuse rights. Record authorization independently of the original repository; transformation does not establish that license or confidentiality obligations disappear. Unknown rights mean tenant-local only.

### Minimum experiment

Use one deliberately synthetic failure shared by two synthetic tenants in an explicitly allowed trust domain. Tenant A encounters a failure; local learn proposes a deterministic check. Create a fresh fixture without copying A's repository content. Tenant B opts into the candidate. Measure detection, false positives on unrelated fixtures, operator review effort and recurrence reduction.

Plant several unique private canaries; promotion fails if a marker or detectable reversible derivative appears in portable artifact, logs, cache or evaluation report. Test a malicious source proposing a gate-weakening rule: reject mechanically, regardless of model verdict. Test unauthorized scope, unknown rights, stale/revoked versions and receiving-policy conflicts.

### Acceptance and stop conditions

Promotion requires independently reproducible portable bundle; denied cross-tenant raw queries; negative disclosure and poisoning tests; no weakened policy or widened authority; deterministic revocation/rollback; measured review cost; separately passed live isolation. These are requirements, not executed test results.

Stop and retain local learning if useful artifacts require source-private context, privacy review requires unauthorized raw access, downstream revocation cannot be traced, rights are uncertain, receiving policy can be weakened or review cost exceeds demonstrated benefit.

**Recommendation:** PR-based promotion workflow only after A04/A07 isolation and #350 experiment/provenance work. No new service or shared database. Compare manual reviewed promotion to tenant-local baseline before automation.

**Confidence:** high in conservative boundary and sequence; medium in eventual value. **Not verified:** live isolation, actual sharing implementation, leakage probability, legal rights for any real artifact, integration tests or CI.
