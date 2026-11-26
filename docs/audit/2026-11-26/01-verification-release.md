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
