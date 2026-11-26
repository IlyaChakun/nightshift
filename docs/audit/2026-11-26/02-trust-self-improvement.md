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

## Decision brief — independent promotion, 2026-11-26

**Recommendation:** keep the existing stable/development split and constitution. Make candidate validation independent of the candidate, separate credentials for promotion, and fix rollback after a failed post-start check before granting autonomous self-promotion. No new service is required for the initial solo deployment.

### Current controls and remaining gaps

| Evidence at pinned base | Disposition |
| --- | --- |
| [ProjectGuard](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/policy/constitution.go#L99-L129) reads protected paths from the base, rejects unreadable policy and combines project rules with built-ins. [Tests](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/policy/constitution_test.go#L57-L129) cover invalid policy and command scripts. | Real existing protection; do not replace it. |
| [ReviewPolicy.Decide](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/policy/review.go#L47-L75) always requires review for critical changes. [Self-tenant rules](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/.nightshift/blast-radius.yaml#L5-L64) classify most code as high or critical, ordinary docs as low within a limit. | Existing risk classification; verify actual effective tenant review configuration. ModeOff is not equivalent to the restrictive self-tenant policy. |
| [commandPaths](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/policy/constitution.go#L131-L180) is a heuristic and explicitly cannot discover all computed or quoted script paths. | Documented limitation; use explicit protected paths and independently controlled check definitions, not an attempt to parse every shell script. |
| [upgrade.sh](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/deploy/upgrade.sh#L201-L239) exports candidate sources, runs their drill and development stack, and links the development stack to local secrets. | Operational smoke checks are useful, but candidate-owned checks and access to actual credentials are not an independent trust boundary. Verify which credentials are reachable, and use synthetic scoped credentials for candidate evaluation. |
| [stable.sh](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/scripts/stable.sh#L58-L101) exports tagged releases, detects a moved cached tag, rebuilds and checks the stable stack. [upgrade.sh](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/deploy/upgrade.sh#L251-L288) rolls back when startup fails; later service/log checks call die without the same automatic rollback. | Existing recovery mechanism, with a concrete post-start recovery gap. Inspect cleanup and test it explicitly. |
| [verify workflow](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/.github/workflows/verify.yml#L15-L44) runs reduced checks for docs-only changes, including agent instructions. | A docs-only classification is not a trust classification. Review prompt/policy changes by effect. Actual host rules, bypass actors and trusted status producers remain unverified (branch protection read denied). |

### Industry evidence

- [GitHub available rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets), read 2026-11-26: branch/tag update restrictions, required checks and review policies support a host-level boundary. Bypass actors must be inventoried. Public repo branch/tag rulesets are available; do not assume private push-rule features are available here.
- [GitHub Actions secure use](https://docs.github.com/en/actions/reference/security/secure-use), read 2026-11-26: least privilege, reviewing workflows and handling untrusted checkouts justify separating candidate evaluation from credentialed promotion.
- [SLSA artifact verification](https://slsa.dev/spec/v1.2/verifying-artifacts), approved specification, read 2026-11-26: a trusted verifier checks the identity of the artifact and its provenance against independently configured expectations. Apply the principle without claiming SLSA compliance.

### Decision and authority model

| Identity | Permitted | Not permitted |
| --- | --- | --- |
| Factory Builder | Candidate patch and job output | Release credential, holdout answers, modifying authority |
| Candidate evaluator | Read candidate, execute in isolated test environment, publish evidence | Production secrets, changing promotion policy |
| Trusted promoter / owner | Authorize a specific candidate digest against trusted policy; deploy or roll back | Blindly accepting a candidate's self-reported checks |
| Emergency operator | Restore previous known-good version outside the candidate | Unlogged policy changes |

The matrix is a proposed target, not a claim about current tokens. Distinguish source merge, evaluation and stable activation. For the first implementation use a small owner-controlled promotion wrapper and policy outside the candidate checkout, protected tag creation and a scoped test environment. Bind evidence to commit, artifact/image digest, evaluator and policy revision. The owner retains authorization for weakening protections; the trusted version or external controller, not candidate code, determines whether the evidence is valid.

Compare three alternatives: (1) current owner-run stable script plus hardening, low cost and recommended now; (2) separate independently owned CI policy repository, useful when several operators or untrusted contributors require stronger enforcement; (3) automated dedicated promotion controller, only after scale and threat model justify it. Signed attestations add value when artifacts cross independently managed trust boundaries, not automatically for a single local operator.

### Implementation sequence and tests

- P0: inventory effective permissions and bypass identities; distinguish operator-owned upgrade entrypoint from candidate-run checks. Do not store secrets or exact private grants in the public research record.
- P0: use disposable evaluation configuration and scoped sandbox credentials. A candidate must not be able to change policy, access stable secrets or publish its own authorization.
- P1: bind a promotion decision to the evaluated revision/artifact and keep the known-good release pointer separate from the candidate pointer until independent health checks pass.
- P1: test failed startup, late service failure, unhealthy smoke, interrupted promotion and incompatible migration. Roll back through the external trusted path and verify the previous version actually serves traffic; if rollback fails, halt new work and surface the operator action.
- Later: evaluate independent workflow/check identity and signed artifacts if external distribution or multiple operators justify them.

**Decision gate:** do not expand autonomy for changes to the trust root until the planted candidate-policy regression, denied credential/authority tests and post-start rollback exercise pass. No need to redesign the learning pipeline; connect the existing fix ladder and polygon to this boundary. Coordinate with A01 and A04.

**Checked:** pinned source/tests above, design section 10, AGENTS.md and the three primary references. **Inferred:** proposed identity split and impact of candidate evaluation. **Confidence:** high for the inspected code paths, medium for effective deployment security. **Not verified:** local tests, actual credentials, live upgrades, full effective host rules.
