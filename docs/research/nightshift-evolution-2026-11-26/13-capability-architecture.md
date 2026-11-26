# Extensible capability architecture

Scale Nightshift through composable capabilities built on its existing packs, skills, roles and adapters. Capabilities describe what a tenant can do and how it is verified; they are not necessarily agents, services or a new plugin framework.

**Priority:** P3. **Status:** research proposal, not approved implementation.

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
- [Deep research decision brief — composition before plugins](#deep-research-decision-brief--composition-before-plugins)

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

## Deep research decision brief — composition before plugins

**Date:** 2026-11-26. **Disposition:** P3; retain existing composition until measured repetition justifies a manifest.

**Provenance:** transferred and edited from the completed Deep Research report, "Nightshift deep research: final block", 2026-11-26. The research session lacked GitHub write access. Subsequent connector check pinned main to 9473fa4bc294825c110b6ec236da7110d0ff0e15; the report's impression of a newer main was not confirmed. External references are report excerpts, not fresh full-text verification in the committing session.

**Decision:** treat existing packs, skills, roles, adapters and project configuration as the capability system until measured duplication proves a missing abstraction. Do not add dynamic Go plugins, an extension registry, WASM runtime or generic capability service now.

### Evidence and alternatives

| Claim | Classification |
| --- | --- |
| Nightshift has packs, skills, role configuration and adapters | Documented and supported by existing project configuration |
| General capability manifest exists | Proposal only; not an implemented contract |
| Existing composition is too repetitive | Not measured |
| Dynamic third-party executable extensions are required | Not demonstrated |
| Agent Skills provides compatible packaging | External specification |
| MCP authorization makes arbitrary tools safe | Rejected assumption |
| WASM can narrow an executable interface | External architecture pattern; runtime isolation and need not established |

Repository grounding: [design](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/design.md), [project schema](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/schemas/project.json), [tenant schema](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/schemas/tenant.json), [runner context](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/runner/context.go), [harness control research](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/research/harness-control.md), [plan](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/plan.md). These identify existing extension surfaces; no complete implementation/security audit was performed in the Deep Research session.

Primary references from the report (2026-11-26 excerpts):

- [Agent Skills specification](https://github.com/agentskills/agentskills/blob/main/docs/specification.mdx): filesystem bundle with SKILL.md and optional scripts/references/assets, progressive disclosure. Use compatible packaging, not automatic trust. [OpenAI Skills documentation](https://developers.openai.com/api/docs/guides/tools-skills) describes registering skill directories; executable resources increase attack surface.
- [Go plugin documentation](https://pkg.go.dev/plugin): portability, race-detector support, dependency/toolchain compatibility and initialization constraints; static composition or IPC are often simpler. Do not allow tenant-installed native plugins.
- [MCP specification update, 2026-07-28](https://blog.modelcontextprotocol.io/posts/2026-07-28/) and [OWASP MCP security](https://cheatsheetseries.owasp.org/cheatsheets/MCP_Security_Cheat_Sheet.html): authorization/routing interoperability does not establish tool trust. Prevent confused-deputy, cross-origin escalation and malicious descriptions. Pin the protocol version actually used.
- [SLSA 1.2](https://slsa.dev/spec/v1.2/) and [Sigstore Cosign](https://docs.sigstore.dev/quickstart/quickstart-cosign/) distinguish artifact identity, provenance and verification. A verified signature is not proof of safe behavior. Use the integrity pattern before adopting a dependency.
- [WebAssembly Component Model worlds](https://component-model.bytecodealliance.org/design/worlds.html): explicit imports/exports enable narrower interfaces. Host grants, runtime vulnerabilities, resource limits and side effects still require evaluation. No current need demonstrated.

### Three capability layers

Knowledge capability: instructions, references, templates and deterministic helper scripts. Prefer Agent Skills-compatible structure where practical. Installation is reviewable, version-pinned and read-only at runtime. Script execution is separately authorized, not implied by text availability.

Integration capability: external system access via existing typed Nightshift adapter or MCP endpoint. Permissions, data classes and side effects are declared independently from natural-language tool descriptions. The model never decides its own authorization.

Execution capability: code running with nontrivial authority. Keep it first-party in the binary or isolated worker image until a real external extension requirement justifies an execution boundary.

A future manifest should describe composition rather than execute code: ID, immutable version, source/digest/provenance, rights, compatible Nightshift versions, roles/task classes, skills, tools/adapters, requested permissions, data classes, egress, verification suite, conflicts, deprecation/revocation and deterministic resolution order. This is a sketch, not a proposed schema migration today.

Deterministic resolver: the same trusted config, repository revision, Nightshift version and capability set yield the same resolved graph. Persist effective resolution with each job. Capabilities request authority; effective permissions are the intersection of instance, tenant, role, task and request. Missing mandatory prerequisite or denied permission fails closed. Text cannot expand tool access. Bind actual execution to resolved versions and policy; check revocation at irreversible boundaries.

Imported skills and manifests are supply-chain inputs. Pin bytes, review provenance/license, scan executable resources, and make upgrades explicit. Mutable latest is not a reproducible execution identity. MCP metadata and returned content are untrusted input; credentials remain outside model context and call-level authorization remains deterministic.

For executable extensions, prefer static composition, then narrowly scoped subprocess/IPC or isolated container with explicit resource/credential constraints. Evaluate WASM only for a demonstrated portable executable requirement not adequately met by simpler alternatives. Neither subprocess nor WASM is inherently a complete security boundary.

### Minimum experiment

No new schema. Pick two real compositions: one deterministic engineering workflow and one read-only integration. Document required skills, role, adapter/tool access, evidence and negative permissions as presets using existing configuration. Compare example-solo and example-team, recording duplicate config, ambiguous selection, permission leakage, version incompatibility and upgrade friction. No real tenant credentials or new egress without approval.

Only propose a manifest ADR when at least two independently useful presets expose the same unresolved composition/versioning problem. Compare existing pack extension against a thin manifest, including maintenance cost, observability, fallback and migration.

### Negative tests and stopping rules

- Unrelated role cannot access disabled capability resources; unapproved provider/egress, data class and protected path are denied.
- A capability cannot weaken gates/review, change authority or bypass independent verification.
- Same version with changed bytes fails integrity; revoked version cannot start new work; conflicts produce explicit diagnostics, not last-write-wins behavior.
- Malicious tool description cannot grant additional permissions; missing mandatory prerequisite blocks execution; rollback restores previously approved composition.
- Two legitimate presets resolve reproducibly without tenant-specific core branches.

No manifest if documented presets suffice; no registry before multiple independent producers; no plugin runtime while first-party execution meets needs; no WASM without concrete measurable justification. Retain Go/Postgres architecture.

**Confidence:** high that runtime abstraction is premature, medium in future manifest shape. **Not verified:** preset runs, tool-scope enforcement, compatibility or security of any new executable extension, integration tests or CI.
