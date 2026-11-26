# Trusted factory upgrades and policy authority review

Preserve the owner-run, tagged upgrade. The clearest operational improvement is recovery after a release starts and subsequently fails its health checks. Effective GitHub authority needs a separate inventory before anyone can judge autonomous promotion.

Reviewed PR #333 against `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition on main |
| --- | --- |
| A candidate can redefine all review rules | Overbroad. ProjectGuard reads the base version and combines its protected paths with built-ins. Critical changes require review in every mode. The self-tenant classifies most code as high or critical. `internal/policy/constitution.go:110`, `internal/policy/review.go:49`, `.nightshift/blast-radius.yaml:6`. |
| Protection depends on what an executable change affects | Confirmed as a design constraint. Source-path inference has documented limits. The reduced CI path is selected by filename. `internal/policy/constitution.go:205`, `.github/workflows/verify.yml:35`. |
| Stable keeps a known revision and can restore it on failed startup | Confirmed. The cached tag is checked against its commit. The upgrade explicitly attempts the previous tag after a failed start. `scripts/stable.sh:59`, `deploy/upgrade.sh:264`. |
| A late failure triggers automatic recovery | The audit correctly distinguishes the two failure windows. The settle checks terminate on bad service state or logs. Cleanup prints a recovery command. `deploy/upgrade.sh:49`, `deploy/upgrade.sh:276`. |
| Candidate evaluation independently authorizes promotion | Unverified. The proposal's identity matrix describes target permissions. A successful drill, repository policy or script inspection cannot establish the effective identity grants. |

**Local reproduction.** A disposable Go test used the existing upgrade fixture to report a stopped service after startup. The script reported the failure. The test expected the previous tag to be started and failed that assertion. The existing late-failure test checks the error message at `deploy/upgrade_test.go:183`.

**Take, phase 6.** Add a disposable upgrade exercise that leaves the old revision serving after a late failure. Verify the running revision as well as the exit code. Include interruption and a failed recovery in the operator-facing outcome.

**Take before phase 7 self-promotion.** Inventory owner, evaluator and promoter permissions privately. Reuse the stable entrypoint, separate the evaluator's disposable configuration, and bind its decision to the evaluated artifact and policy revision. Distinguish authorization to merge source from authorization to activate stable.

**Later, phase 7.** Consider an independently owned policy repository or signing only if more operators or consumers must establish trust independently. **Reject now** a dedicated promotion service.

GitHub allows ruleset exceptions for “certain roles, specific teams, or GitHub Apps”. A repository's protected flag cannot answer the identity question. Read 2026-11-26. [GitHub rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets)

**Checked** Base policy, CI classification, stable scripts and the upgrade cleanup path. **Inferred** Phase and priority. **Confidence** High for script behavior. Effective grants, real upgrade, migration compatibility and live recovery remain unverified.
