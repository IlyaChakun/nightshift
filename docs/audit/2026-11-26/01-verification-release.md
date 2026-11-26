# NS-A01 — Verification and release integrity

**Priority:** P0 for fail-open gates and release authorization; P1 for evidence provenance and coverage.  
**Status:** candidate findings; validate at current HEAD.

## A01.1 — A missing verification result may be treated as success

**Earlier review observation:** [gate.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/gate.go#L24-L60) and [verify.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/verify.go#L21-L29) appeared to allow progress when no Verifier was configured. The reviewed acceptance path appeared to enter `Accepted` without a distinct acceptance Verifier invocation. This may be deliberate for development profiles; inspect current policy and downstream guards before classifying it as a production defect.

**Why it matters:** `not run`, infrastructure failure, explicit policy exception and successful verification are different facts. Collapsing them can authorize release without evidence. A disconnected dependency should not silently raise confidence.

**Invariant:** every release has a current successful result for every required check, or a separately authorized policy exception if that check is explicitly waivable. A non-waivable gate never advances without success.

**Options and trade-offs:**
- Minimal: reject missing, failed, stale or unavailable required verification at the release boundary; fail startup for invalid production configurations. Low implementation cost; scattered rules can drift.
- Typed gate result (`PASSED`, `FAILED`, `NOT_RUN`, `SKIPPED`, `UNAVAILABLE`) and one policy evaluator. Stronger semantics, easier audit and testing; requires migration of callers and existing state. `SKIPPED` must carry policy basis, not mean pass.
- Separate dev/test and production policies. Preserves rapid local work but requires configuration validation and tests for accidental use of permissive profiles.

**Suggested approach:** typed outcomes plus centralized release eligibility, introduced incrementally. Preserve permissive modes only where explicitly configured and unable to release to production.

**Acceptance tests:** no verifier; unavailable verifier; empty/malformed verdict; timeout; failed mandatory acceptance; explicit allowed skip; prohibited skip; success. For each negative case assert no release side effect and a durable, diagnosable blocked state.

## A01.2 — Release approvals may not be exercised through the real authorization path

**Earlier review observation:** [scenario_integration_test.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/scenario_integration_test.go#L97-L113) appeared to advance conductor for non-spec cards without exercising the user action handler. That is a test coverage concern, not evidence of an authorization bypass in production.

**Invariant:** merge/deployment approvals are authenticated, authorized, scoped to the intended action and artifact, single-use or idempotent, and invalidated when relevant inputs change. The release boundary independently enforces this.

**Options:** extend scenario integration tests through actual callback routing and authorization with fake transport (fast, deterministic, misses external provider behavior); add a real sandbox Slack/GitHub integration run (higher confidence, slower and operationally costly); add both at different CI tiers.

**Acceptance tests:** unauthorized actor, replay, duplicate callback, stale approval after SHA change, simultaneous cancel/merge, crash after approval, approval for wrong task/environment, missing acceptance result. Verify no unauthorized external effect. Document ordering semantics for conflicting valid actions.

## A01.3 — Verification evidence may become stale after rebase or merge

**Earlier review observation:** [build.go](https://github.com/IlyaChakun/nightshift/blob/main/internal/dispatch/build.go) included SHA-256 checks for patch artifacts. The earlier audit did not establish whether every release evidence type is bound to the exact final artifact.

**Invariant:** the released artifact is the artifact actually verified, under the applicable specification, policy and evaluation inputs. A changed input invalidates affected evidence. A Git commit alone may be insufficient if build output or dependencies vary.

**Options:** persist an immutable evidence manifest with task/spec revision, candidate and base commit, artifact digest, required policy revision, scenario suite digest, runner/toolchain identity and outcome. Re-run affected checks when inputs change. This improves traceability but adds retention and invalidation costs. For stronger assurance use signed attestations from a distinct verifier identity; signing adds key management and is unnecessary if the simpler trusted boundary suffices initially.

**Suggested approach:** start with immutable DB records and digest checks at promotion; add external attestation where threat model requires it. Verify the merge result (or equivalent reproducible artifact) rather than assuming pre-rebase evidence applies. Keep hidden scenario contents private, exposing only safe identifiers/digests.

**Acceptance tests:** rebase, new commit, changed scenario suite, changed policy, changed build artifact, stale approval, substituted artifact. Each must block promotion or trigger correct re-verification. Demonstrate happy-path release consumes exactly the authorized artifact.

## Follow-up source check and retention

This dated external review is a candidate finding record, not the living design or a verified defect register. Preserve the original findings and append a disposition or link to implementation evidence; do not delete this record merely because an issue is resolved. Superseding or relocating it requires an explicit owner decision.

**Checked:** pinned Nightshift base [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26. At the pinned base, `internal/dispatch/gate.go` lines 28–45 and `internal/dispatch/verify.go` lines 23–29 explicitly allow a missing Verifier; the former still contains the preview/acceptance gate comment. Recheck downstream production policy, actual deploy path and open browser-acceptance PR #329 before concluding exploitability. The scenario test has changed; verify the callback path afresh.

**Inferred:** proposed priorities, failure modes and remedies in this document; no tests or live runs performed. **Confidence:** medium for the risk assessment, not a confirmed incident.

## Decision brief — deeper review, 2026-11-26

**Recommendation:** accept A01 as a release-readiness workstream, not as a proven production incident. Fix required-check semantics and final-candidate authorization before enabling autonomous production releases. Reuse the existing review policy, state machine, book and merge train. Do not add an external workflow engine or signing service merely to solve this gap.

### Findings and disposition at the pinned base

| Finding | Disposition | Evidence |
| --- | --- | --- |
| A01.1: missing verification | Confirmed static behavior; production impact conditional | [gate.go](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/gate.go#L28-L65) advances acceptance without a distinct verdict and [verify.go](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/verify.go#L23-L50) deliberately permits an absent verifier configuration. [preview](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/gate.go#L67-L109) does distinguish a failed smoke and platform error. |
| A01.2: approval coverage | Partially confirmed; not an absence of all authorization | [scenario runner](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/scenario_integration_test.go#L101-L117) substitutes a tick for a non-spec click. [Inbox](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/chat.go#L65-L74) checks actor authority; [humanReview](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/gate.go#L207-L249) checks eligible GitHub reviewers and current head SHA; [review tests](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/reviewauth_integration_test.go) cover outsider decisions. The separate production approval path described in design needs implementation/verification. |
| A01.3: final-candidate evidence | Confirmed missing enforcement in inspected train; effective protection conditional | [train.Run](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/train/train.go#L123-L168) rechecks CI on rebased head before merge; returned approval-retention information does not gate merge inside this function, nor is it used by the [conductor caller](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/gate.go#L373-L388). [Train tests](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/train/train_test.go#L151-L169) check the reported approval state and successful merge, not an independent reapproval requirement. |
| Related policy lookup errors | Additional static finding; classify with owner policy | [holdRelease and quiet](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/gate.go#L391-L421) treat some unknown scheduling or capability results as permission to continue. Distinguish a deliberate operational schedule exception from missing authorization; prefer blocking production when release eligibility is unknown. |

PR [#329](https://github.com/IlyaChakun/nightshift/pull/329) adds browser tooling and a skill. The changed-file list and skill were inspected; no conductor or train file is changed by that PR at inspection time. It is a useful prerequisite, not evidence that acceptance is mandatory or enforced. The phase 6 live acceptance criterion remains open in the checked status. Main is reported protected, but the branch-protection read returned 403 and the rules query was unavailable; effective rules, status sources and bypass identities remain unverified.

### External evidence and applicability

- [OWASP AI Agent Security: high-impact action integrity](https://cheatsheetseries.owasp.org/cheatsheets/AI_Agent_Security_Cheat_Sheet.html), read 2026-11-26. Validate scope and authorization in the execution component, bind approvals to the actual action, and deny on missing or failed security decisions. Apply to the release executor; a model verdict is not authorization.
- [GitHub rulesets: pull requests and status checks](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets), read 2026-11-26. Stale-review dismissal, required recent-push approval and trusted status sources are available controls. Use host enforcement as defense in depth; it cannot represent every tenant-specific acceptance or deployment requirement.
- [SLSA v1.2: verifying artifacts](https://slsa.dev/spec/v1.2/verifying-artifacts), read 2026-11-26, approved specification. Validate provenance against the actual artifact digest and trusted builder/policy expectations. Adopt the binding principle; an internal database record is not a signed SLSA attestation and does not establish SLSA compliance.

### Options and decision

1. **Host rules only:** low implementation effort and valuable independent enforcement, but tenant-specific acceptance and actual deployment artifacts are outside ordinary PR review. Keep as additional protection, not the sole authority.
2. **Existing Go policy plus durable release eligibility evidence (recommended):** explicit outcome per required check, bound to tenant, task, action, environment, candidate/base, spec and effective policy revisions. At the merge/deploy boundary check current required evidence and approval; persist the authorized operation and reconcile retries. No new service. Cost: transition and retry changes, evidence invalidation and regression tests.
3. **Externally signed attestations and artifact promotion:** stronger separation of trust across builders and deployers; introduces identity, key and artifact lifecycle complexity. Defer until a separate trust domain, external consumers or a compliance requirement warrants it.

Keep an explicit non-deploying development profile. A missing optional check may be recorded as not applicable only if permitted by policy; a mandatory check cannot be waived by absence or infrastructure failure. Separate merge authorization from production deployment authorization. Do not treat identical patch IDs as proof that a changed base preserves runtime behavior. If a relevant input changes, obtain fresh evidence; for human review, renew authorization when the policy requires it. An already merged PR is evidence of a merge, not automatically evidence of authorized deployment. In restart/reconciliation paths, recover a previously durable authorization or block and escalate.

### Implementation sequence and verification contract

- **P0 / one small PR:** explicit mandatory-gate eligibility, fail-closed production behavior and tests for missing/failed/unavailable check and unknown release capability. Reuse policy floor and existing non-production behavior.
- **P0 / separate PR:** final-candidate approval and verification binding across merge and deploy, including restart recovery. Coordinate with A03 for durable transaction and side-effect semantics.
- **P1 / separate PR:** actual action-path scenarios through the appropriate authorized handler; retain existing GitHub reviewer tests. Add live twin-contract validation for host controls and the real preview/deployment path.
- **P1 / later:** artifact digest and trusted producer evidence, with independent verifier access controls; consider signing only after a threat-model decision.

Minimum negative scenarios: missing required verifier; required acceptance unavailable; changed candidate or base; stale/foreign approval; duplicate decision; cancel/merge race; crash before and after an external side effect; already-merged PR with no current deployment authorization. Assert the absence of unauthorized merge or deploy calls, not just the task state. For the happy path, assert that the artifact deployed is the approved and verified artifact. For low-risk non-production tasks, explicitly prove the intended policy exception remains functional.

**Decision gate:** retain current architecture. Do not enable production autonomy until mandatory acceptance, approval binding and final-candidate release checks pass both deterministic T2/T4/T5 scenarios and an authorized live sandbox exercise. No claim of exploitation or production exposure is made.

**Checked:** pinned source links above, PR #329 changed files, repository status and the three external primary references. **Inferred:** risk ranking, recommended implementation sequence and conditional production impact. **Confidence:** high for inspected control flow, medium for end-to-end security implications. **Not verified:** tests, actual tenant policy, host enforcement, CI, deployment wiring end to end and live behavior.
