# A07 review — evidence for a staging pilot

Run the smallest staging exercise that closes the outstanding phase 6 deployment check. The audit gives a useful checklist for the pilot, but its fake-only description understates the live evidence already recorded. Keep the first failure and the retest linked to their respective revisions.

Checked revision: `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition | Evidence |
|---|---|---|
| A Mac drill passed 22/22, one chat idea reached release and real Builders produced patches | Confirmed as repository reports | `docs/status.md:11` and `docs/plan/live-checks.md:171`. `scripts/drill.sh:115` checks restore and restart. The drill's scope is infrastructure. |
| Lead and deploy have only fake evidence | Already superseded in part | `docs/plan/live-checks.md:170` records five real Lead intake cases by operator account, without a result file. `docs/status.md:3` records a real Docker run that found defects. Rechecking the fixes is pending at `docs/plan/live-checks.md:15`. |
| CI and fixtures supply complementary coverage | Confirmed | `Makefile:59` uses the race detector and a printed shuffle seed. `.github/workflows/verify.yml:15` narrows checks for docs PRs. `internal/jobsapi/claim_test.go:108` covers fairness. Concurrent tenants and worker-death recovery still await live runs. |
| The polygon has only a greeting task | Partly | The Builder directory has `evals/tasks/greeting/task.yaml:1`. Lead has separate fixtures at `evals/lead/cases.yaml:10`. `internal/evals/run.go:120` grades a local task. Browser tooling in the image supplies no staging result. |

**Take — phase 6 P4b/P5/P6.** Extend the existing live-check record with revision, task, stack, harness, expected and observed outcomes, time, human interventions, cost and cleanup. Run one synthetic request through configured roles, preview, independent acceptance and an authorized staging deployment. Retain failed smoke, rollback and teardown evidence. Add repeatable checks for missing crash, cancellation, duplicate and recovery cases to the existing phase 3 X2 matrix. Check adapter contracts against disposable endpoints in the authorized environment.

**Take — phase 7.** Freeze comparable tasks, graders and holdouts. Measure verified delivery, first-pass acceptance, required approvals versus unplanned assistance, rework, observed escapes, elapsed time and total cost per accepted objective. Include failures, abandonment, cohort size and observation period. Use recorded outcomes to decide whether an independently evaluated upgrade deserves promotion. Anthropic defines outcome as “the final state in the environment at the end of the trial”. Read 2026-11-26. [Agent evals](https://www.anthropic.com/research/demystifying-evals-for-ai-agents)

**Later — phases 6–7.** Add holdout rotation, property, mutation, differential or metamorphic tests, sampled trajectories, integrated contract checks and canaries where a missed defect justifies each. Assess signed attestations only for a documented provenance requirement. **Reject — current pilot.** A second metric store, paid fault platform, broad mutation campaign or a readiness score based on one success.

**Checked** — local `TestTheCacheKeyFollowsTheTaskAndTheCandidate` passed. Read status and live ledger, drill, CI, eval fixtures and grader, design §20. **Inferred** — staging order and measurement priorities. **Unverified** — historical run artifacts, CI execution, live integrations, staging rollback, production readiness and pilot economics. **Confidence** — high for inventory, medium for operational state.
