# NS-A02 — Trusted self-improvement and policy authority

**Priority:** P0 before autonomous self-promotion.  
**Status:** architecture risk; actual permissions and GitHub rules were not inspected.

## Observation and risk

The earlier review of [AGENTS.md](https://github.com/IlyaChakun/nightshift/blob/main/AGENTS.md) indicated that agents could propose and, under certain review rules, merge changes affecting workflows, deployment, schemas, Makefile, agent instructions and dependencies. Such changes are legitimate engineering work. The concern is whether the same identity can also alter or bypass the independent controls that authorize those changes. A written prohibition is not equivalent to enforced authorization.

Potential failure modes include accidentally weakening CI, replacing the verifier, altering a required check, changing deployment credentials or promoting a candidate that evaluated itself under newly weakened rules. No actual bypass has been demonstrated.

## Required invariants

- A candidate factory version cannot authorize its own promotion using policy or verification logic it just changed.
- Builder credentials cannot modify external rulesets, release authority, protected evaluation inputs or production credentials.
- Critical policy changes are subject to an independently enforced authorization path. The last known good version remains recoverable.

## Design alternatives

| Option | Benefits | Trade-offs |
|---|---|---|
| GitHub rulesets, protected required workflows/check identities, scoped app credentials and protected environments | Reuses platform controls; low initial infrastructure burden | Requires checking effective permissions, bypass lists and plan support; a check name alone may not establish independent authority |
| External policy/verification repository with independently controlled runner | Stronger separation; versioned policy and evaluation inputs | More repositories, integration and maintenance; control-plane credentials still need protection |
| Dedicated promotion service/controller | Can enforce artifact provenance, canary and rollback automatically | Additional operational component and failure modes; may be premature |

## Recommended incremental approach

Inventory identities, permission grants and actual external protection configuration. Classify sensitive changes by effect rather than only path (e.g. dependencies or reusable actions can alter gate behavior indirectly). Define a minimal independently protected policy root and run the current trusted factory against a candidate artifact. Require explicit owner/independent authority for policy-root changes; keep routine product code autonomy high. Use staged candidate deployment, scoped credentials, health checks and rollback to a known-good digest. Consider stronger promotion infrastructure only if the initial boundary cannot enforce the invariants.

## Acceptance criteria

- Demonstrate a candidate cannot self-authorize by changing its workflow, verifier, check name, dependency or policy configuration.
- Demonstrate agent token cannot change rulesets, protected secrets, evaluation access or production authority.
- A policy change requires an independent authorized approval; bypass permissions are documented and tested where feasible.
- Candidate promotion uses an immutable artifact evaluated by the prior trusted version or external evaluator.
- Simulate a failed canary and recover to last known good version without depending on the broken candidate.
- Produce a threat model and permission matrix, separating actual verified controls from assumptions.

**Dependency:** NS-A01 evidence and release checks; NS-A04 identity and sandbox isolation; NS-A07 evaluation baseline.

## Follow-up source check and retention

This dated external review is a candidate finding record, not the living design or a verified defect register. Preserve the original findings and append a disposition or link to implementation evidence; do not delete this record merely because an issue is resolved. Superseding or relocating it requires an explicit owner decision.

**Checked:** pinned Nightshift base [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26. At the pinned base, root `AGENTS.md` permits agents to edit and merge gated paths with independent review and explicitly requires owner approval for weakened protections. `main` reports protected; exact rules and independent trust enforcement are not checked. Treat this as a threat-model and assurance question.

**Inferred:** proposed priorities, failure modes and remedies in this document; no tests or live runs performed. **Confidence:** medium for the risk assessment, not a confirmed incident.
