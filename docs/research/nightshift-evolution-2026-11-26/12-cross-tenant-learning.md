# Cross-tenant learning and knowledge promotion

Reuse validated generic engineering lessons across eligible tenants without transferring their private context. Begin with an opt-in, human-reviewed promotion path for synthetic rules and tests; personal and work trust domains remain separate.

**Priority:** P2. **Status:** research proposal, not approved implementation.

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
