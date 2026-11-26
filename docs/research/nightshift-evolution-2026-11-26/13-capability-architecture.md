# Extensible capability architecture

Scale Nightshift through composable capabilities built on its existing packs, skills, roles and adapters. Capabilities describe what a tenant can do and how it is verified; they are not necessarily agents, services or a new plugin framework.

**Priority:** P3. **Status:** research proposal, not approved implementation.

## Problem

As new products require browser workflows, data engineering, operations, security or discovery, ad hoc changes can accumulate in the core. Creating a dedicated agent for every skill also increases coordination cost. Unreviewed extensions may expand tool access, disable gates or introduce incompatible dependencies.

## How it works

Treat a capability as a proposed composition of versioned skills, applicable roles, stack tools, verification contracts, input/output evidence and permission requirements. A tenant opts into approved compositions through existing configuration and pack mechanisms. Lead, Builder and Verifier receive only the subset relevant to the current task, with deterministic policy enforced by Nightshift.

A conceptual manifest records identifier, version, purpose, prerequisites, supported task classes, required tools, permissions, data classification, verification suite and compatibility. It is a design sketch, not a new implemented schema. Resolution should be deterministic, record provenance and fail clearly if requirements are unmet. Core security and tenant policies always take precedence.

## Trade-offs

| Option | Benefit | Limitation |
| --- | --- | --- |
| Existing packs, roles and skills as-is | No new abstractions | Repeated composition and unclear compatibility |
| Documented capability presets using existing fields | Small, reusable step | Limited discovery and validation |
| Versioned declarative manifests | Explicit dependencies and lifecycle | Schema evolution and resolution complexity |
| Arbitrary executable plugin runtime | Flexible | Untrusted code and permission expansion; defer |

Start with a documented preset and one real need. Only introduce a manifest when multiple presets demonstrate recurring composition or compatibility problems. Avoid a registry service, dynamic executable plugins and new mandatory agent roles.

## Sources

- Design sections 9, 12 and 17, `docs/research/harness-control.md` and `docs/prior-art.md`: existing pack, skill and provider abstraction research.
- [NIST SSDF 1.1](https://csrc.nist.gov/pubs/sp/800/218/final), search excerpt checked 2026-11-26: reference for secure development and component provenance.
- Prior PR #340: proposed tenant-scoped Product Intelligence; PR #346: production operations as another potential capability. Neither is an approved implementation contract.

## Fit

Existing stack packs handle tools, tenant packs handle resources and governance, project configuration supplies instructions and skills, and harnesses isolate vendors. Preserve this layering. A capability cannot grant itself authority, add egress, change protected policy or embed tenant-specific names in core.

## Status

Partial foundation: pack, skill, config validation and role override mechanisms are documented in the plan. A generalized capability composition and compatibility contract is a proposed extension; no claim is made that such a manifest exists.

## Proposal

Pilot two compositions without changing the core: backend contract verification using available tools and product discovery in advisory mode. Document prerequisites, role assignments, policy constraints, evidence outputs and acceptance tests using existing mechanisms. Record friction and duplicate configuration.

If repeated demand warrants a declarative representation, compare existing pack extension against a thin capability manifest in an ADR. Validate versions, dependencies and conflicts at onboarding. Test upgrade and rollback, pin imported content, scope tools by role and tenant, and preserve independent verification. Distinguish required checks from optional advisory behavior; missing mandatory prerequisites must block rather than silently weaken validation.

## Acceptance and stop conditions

- Two synthetic tenants select different capabilities without tenant-specific core branches.
- Disabled capability tools and data are not available to an unrelated job.
- A missing prerequisite, incompatible version or forbidden permission produces a clear error.
- An extension cannot lower the existing gate or data-classification floor.
- Upgrade and rollback are reproducible and independently testable.
- No new manifest or plugin runtime is justified if documented presets meet the observed needs.

## Agent follow-up

Review PRs #340, #344, #346 and the existing pack/config contracts. Prioritize first-product needs and phase 10 onboarding. Treat broader capability distribution as a later research topic, not an immediate refactor.

## Record and evidence

Preserve this dated owner-requested proposal. Link later experiments, decisions and implementation PRs; do not silently delete or replace the source record. The living design remains `docs/design.md` and the canonical sources index is `docs/prior-art.md`.

**Checked:** [base 9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; design sections 4, 5, 6, 9, 12, 16, 17, 22, plan, status, research index and open PR titles. **Inferred:** candidate gaps, priorities and experiment design. **Confidence:** medium; no live validation or full code audit. External links identified above were checked only to the extent stated.
