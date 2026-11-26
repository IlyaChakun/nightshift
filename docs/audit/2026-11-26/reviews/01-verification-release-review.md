# Verification and release integrity review

Phase 6 needs evidence for the exact change that an owner authorizes for deployment. Nightshift already checks reviewer identity and revision for GitHub reviews. The audit identifies work that belongs in the existing release flow.

Reviewed PR #332 against `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Audit claim | Disposition on main |
| --- | --- |
| A01.1 and the related release-policy observation | Confirmed, details kept outside the repository. |
| A01.2, approval coverage in the scenario suite | Partly confirmed. The scenario helper models a deploy-card click by advancing the conductor. The spec-card path calls the inbox, which checks the actor. Separate GitHub review tests cover outsiders. See `internal/dispatch/scenario_integration_test.go:101`, `internal/dispatch/chat.go:65` and `internal/dispatch/reviewauth_integration_test.go:12`. |
| A01.3, final-candidate evidence | Confirmed, details kept outside the repository. |
| Production impact | Unverified. An observed transition in a twin cannot establish the deployed policy, effective GitHub rules or a production side effect. |

**Take, phase 6.** Extend P5 and its T4 scenario with one acceptance record that identifies the candidate and the policy requiring the check. Exercise an authorized decision through the real handler on a fake transport. Verify which revision the deployer actually received, including after a restart. Keep merge and production authorization explicit in the test's expected events.

**Later, phase 6.** Expand evidence to artifact and toolchain digests when build and deployment can identify the artifact independently. Review host rules and run the documented live release exercise with the owner before expanding autonomy.

**Reject for this change.** Host rules alone cannot express Nightshift's tenant-specific deployment requirements. A signing service would add an operational dependency before a second trust domain needs to verify these records.

GitHub documents “approval of the most recent reviewable push” as a configurable rule. Its availability establishes no effective setting for this repository. Read 2026-11-26. [GitHub rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets)

**Checked** Current conductor, approval scenarios and phase 6 P5. The existing clean merge and failed-smoke rollback train tests passed on twins. **Inferred** Implementation order. **Confidence** High for source behavior. Production deployment, live approval and effective host settings remain unverified.
