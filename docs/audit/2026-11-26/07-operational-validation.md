# NS-A07 — Operational validation and continuous improvement

**Priority:** P1 for live end-to-end and recovery proof; P2 for quality analytics and advanced evaluation.  
**Status:** validation and measurement roadmap.

## Contents

- [Earlier review observations](#earlier-review-observations)
- [Validation plan](#validation-plan)
- [Quality metrics](#quality-metrics)
- [Evaluation techniques to investigate and selectively apply](#evaluation-techniques-to-investigate-and-selectively-apply)
- [Controlled self-improvement](#controlled-self-improvement)
- [Acceptance criteria](#acceptance-criteria)
- [Follow-up source check and retention](#follow-up-source-check-and-retention)
- [Decision brief — evidence-based readiness, 2026-11-26](#decision-brief-evidence-based-readiness-2026-11-26)

## Earlier review observations

The earlier audit read [docs/status.md](https://github.com/IlyaChakun/nightshift/blob/main/docs/status.md) as reporting completed early phases, 22/22 local recovery drills, one successful end-to-end flow to `released`, real Claude/Codex Builder runs, and several full-flow, deployment, hidden-scenario, worker-death or multi-tenant paths still relying on fakes/stubs or pending live validation. These are historical reported results, not independently reproduced measurements; re-check current status and test evidence before prioritization.

The risk is not that abstractions or fake tests are inherently bad. It is that architecture can evolve faster than evidence that the complete system works under realistic conditions.

## Validation plan

Use a disposable external product repository and isolated staging infrastructure, with non-production accounts and explicit budget limits. Choose a representative product with multiple dependent tasks, persistence, API behavior and deployable artifact. Keep acceptance cases outside Builder access. Execute the real Lead/Builder/Verifier workflow through preview, acceptance, approval and deployment. Capture provider/harness versions, source revision, artifact digests, environment, costs, human interventions and actual outcomes.

Repeat with worker death, database interruptions, duplicate callbacks, provider errors, cancellation, competing changes and invalid evidence. Exercise real Slack/GitHub paths in sandbox environments when those integrations are required. Record negative results rather than silently repairing the product manually. Promote successful fixes to targeted regression tests. No production deployment or real-customer credentials are necessary for this validation.

## Quality metrics

| Metric | Definition / caution |
|---|---|
| Autonomous completion rate | Verified objectives completed without human action / eligible attempted objectives; report required policy approvals separately |
| First-pass acceptance | Objectives accepted on first complete candidate / evaluated objectives |
| Human intervention rate | Objectives needing unplanned human assistance / attempted objectives |
| Defect escape rate | Accepted objectives with confirmed escaped defects / accepted objectives observed for a defined period |
| Cost per accepted task / feature | Total attributable model and execution cost including failed attempts / accepted units at each level |
| Rework ratio | Repair effort or cost / total measured engineering effort or cost; define units |
| Recovery success rate | Fault-injected or actual recoverable failures restored within defined invariant and time bounds / attempted recoveries |
| Specification fidelity | Independently satisfied required criteria / applicable criteria; report critical failures separately |
| Time to verified delivery | Elapsed time from accepted input to verified output; report distribution and abandoned work separately |

Track cohort, complexity, policy profile, model and harness version. Report sample size and failure cases; avoid conclusions from one successful run. Use existing OpenTelemetry/Grafana infrastructure where available. Maintain both operational and product-outcome metrics; never optimize a metric by weakening eligibility or excluding failures without disclosure.

## Evaluation techniques to investigate and selectively apply

| Technique | What to test / trade-off |
|---|---|
| Independent acceptance and holdout rotation | Prevent overfitting to visible checks; preserve reproducibility and isolate evaluator inputs |
| Digital twins / contract simulation | Exercise external dependencies cheaply; periodically validate fidelity against sandbox or real contracts |
| Mutation testing | Detect weak assertions; expensive for whole codebase, start with gates and critical transitions |
| Property-based and model-based testing | Verify invariants across many transitions; invest in accurate model and actionable counterexamples |
| Deterministic simulation and fault injection | Reproduce concurrency and crash boundaries; complement with real infrastructure tests |
| Differential and metamorphic testing | Check equivalent behavior and invariants when exact expected outputs are difficult; select sound relations |
| Agent trajectory evaluation | Diagnose tool usage and repeated failure; avoid collecting unnecessary sensitive prompt/trace data |
| Semantic merge conflict testing | Detect behavior conflicts after clean merge; use integrated acceptance and contract tests first |
| Evidence attestation | Bind quality evidence to released artifact; adopt independently signed proof only when threat model warrants it |
| Progressive autonomy and canary upgrades | Expand permissions based on demonstrated reliability, with reversible deployment and trusted promotion |

Read existing [prior art](https://github.com/IlyaChakun/nightshift/blob/main/docs/prior-art.md) and [StrongDM techniques](https://factory.strongdm.ai/techniques) before duplicating research. For each proposed technique document applicability, implementation status, measurable benefit, limitations and evidence. Favor proven local improvements over importing entire frameworks.

## Controlled self-improvement

Treat factory changes as experiments: freeze an evaluation baseline, define target metric and safety invariants, compare candidate with trusted version on representative and held-out tasks, record cost/quality trade-offs, and promote only through NS-A02's independent authority. Retain rollback and monitor regressions. A higher task completion rate does not justify weaker verification or increased unauthorized access.

## Acceptance criteria

- A reproducible staging run produces a verified deployable artifact through the actual configured integrations, with a complete evidence trail.
- Multi-task acceptance, hidden evaluation isolation, cancellation, worker-death recovery and duplicate delivery are exercised at the appropriate tiers.
- A documented baseline includes actual results and limitations, not fake/stub success presented as live success.
- A subsequent improvement is evaluated against the same baseline and a held-out scenario set; compare cost and quality, and demonstrate no weakened invariants.
- Remaining unsupported operational claims are explicitly marked unverified. Establish production-readiness criteria based on risk and measured reliability rather than an arbitrary numeric score.

## Follow-up source check and retention

This dated external review is a candidate finding record, not the living design. Preserve the original findings and append a disposition or link to implementation evidence; do not delete this record merely because an issue is resolved. Superseding or relocating it requires an explicit owner decision.

**Checked:** pinned Nightshift base [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; `docs/status.md` and `docs/research/README.md`. Current status records 22/22 Mac drills, one released chat idea, live Builder patches, fake-only Lead, stub deployment, and outstanding live multi-tenant and worker-death checks. Reconcile with latest evidence before piloting.

**Inferred:** priorities and proposed solutions; no tests or live runs performed. **Confidence:** medium.

## Decision brief — evidence-based readiness, 2026-11-26

**Recommendation:** finish an explicit live-readiness matrix before expanding features or raising autonomy. Keep T1–T8 and E1–E4 as the existing architecture; add missing cases and reproducible evidence rather than a second testing platform. Do not mistake a green docs PR, fake scenario or isolated infrastructure drill for product-delivery proof.

### Evidence and limits

| Evidence at pinned base | What it establishes / does not establish |
| --- | --- |
| [Status](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/status.md#L9-L32) reports 22/22 Mac drills, one released idea, live Builder patches, fake-only phase 5, stub deployment and pending live checks. | Historical repository report, not independently reproduced; obtain the associated run evidence. |
| [drill.sh](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/scripts/drill.sh#L91-L158) checks disposable database migration, backup/restore, restart and agent/egress boundaries; blocked tests require a positive end marker. | Useful live infrastructure proof, not a multi-agent delivery, deployment or concurrent tenant recovery test. |
| [Makefile](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/Makefile#L36-L38) runs Go tests with race detection and a recorded shuffle seed; [verify.yml](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/.github/workflows/verify.yml#L15-L44) intentionally reduces docs-only checks. | Useful deterministic baseline. Check individual test results and skip reasons, not merely aggregate green status. |
| [Scenario directory](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15/testdata/scenarios) contains happy-path, changes-requested and spec-rejected YAML files; [claim tests](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/jobsapi/claim_test.go) cover tenant fairness, provider caps and lease sweep. | Existing coverage to retain; directory count is not the total number of integration tests. Actual process death and concurrency on the target stack remain unverified. |
| [Polygon tasks](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15/evals/tasks) contains greeting at inspection; [ADR 0011](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/adr/0011-polygon-runs-locally.md) describes local grading and explicitly excludes live push/PR proof. [Selection](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/evals/selection.yaml) maps changes to evals. | Very limited observed task diversity; inspect any additional Lead fixtures and eval paths before claiming the total suite size. |
| [Browser acceptance PR #329](https://github.com/IlyaChakun/nightshift/pull/329) adds tools/skill, not proof of live deployment. | Coordinate with A01; execute actual mandatory acceptance and verify denial cases. |

### Industry research

- [Anthropic, Demystifying evals for AI agents](https://www.anthropic.com/research/demystifying-evals-for-ai-agents), published 2026-01-09, read 2026-11-26: distinguish task, trial, grader, trajectory and actual environment outcome; repeat stochastic trials, separate capability and regression tests, prefer code graders where possible and calibrate model judges. The article reports practitioner experience, not Nightshift results.
- [Google SRE, Testing for Reliability](https://sre.google/sre-book/testing-reliability/), read 2026-11-26: layer inexpensive tests with integration, system and production observations. Passing a test reduces uncertainty, not a guarantee of reliability.
- Existing Antithesis/FoundationDB and StrongDM entries in prior-art provide leads for seeded fault injection and hidden scenarios. Reuse them; do not adopt a paid external platform before local experiments justify it.

### Readiness matrix for the first controlled pilot

| Gate | Required live or deterministic evidence |
| --- | --- |
| G0 — authority and safety | Resolve applicable A01–A04 blockers; revoke exposed credentials if outstanding; prove no unauthorized merge/deploy and no hidden-input access on a disposable stack. |
| G1 — installation and recovery | Repeat current drills on pinned release; record restore time and recovered data, worker death and lease reclaim, cancellation and process cleanup. No lost logical results. |
| G2 — actual delivery | One raw request through real configured Lead, Builder, independent Verifier, GitHub and preview; test a correction, duplicate and dependency. Capture exact revision and human actions. |
| G3 — real deployment | Authorized staging deployment, independent acceptance, negative smoke/rollback, teardown and actual recovered revision. No real customer data. |
| G4 — repeatability | Repeat representative tasks and failures with bounded budgets; include an unrelated concurrent tenant only inside the agreed trust domain. Preserve all failures and intervention records. |
| G5 — baseline and upgrade | Freeze comparable standalone-harness and Nightshift task cohorts; measure verified outcomes, cost, attention, rework and escapes. Reject a planted degraded candidate through independent promotion. |

These gates are proposed validation criteria, not new runtime commands or automatic permission changes. G0–G3 are prerequisites to unattended staging; production requires separately authorized policy and operational readiness. G4–G5 support later autonomy increases. Do not make a universal success percentage from a tiny pilot sample.

### Implementation sequence and experiment contract

1. Reconcile the current status, open phase 6 PRs and A01–A04 before scheduling a live test. Keep one evidence ledger indexed by release revision, run ID, scenario ID, environment, adapter/harness version, expected invariant, result, artifact reference, duration and cleanup outcome. Use explicit NOT_RUN, PASS, FAIL and BLOCKED dispositions.
2. Extend existing deterministic tests only where the failure matrix lacks coverage. Prioritize mandatory authorization, stale evidence, atomic result consumption, overlapping leases, rollback and recovery before broad mutation testing.
3. Run the smallest authorized disposable live stack exercise. Use synthetic data, scoped credentials, explicit budget, no paid calls without approval, and record the first failure before fixing. Repeat failed scenarios after a fix; retain initial results.
4. Grow the polygon from actual escaped failures and representative task classes: intake ambiguity, multi-file feature, regression, API contract, migration, and verifier rejection. Protect the holdout from candidate access. Freeze grader and input versions; report per-task outcomes and variability.
5. Compare baseline/candidate at the outcome level, including incomplete work and operator minutes. Use PR #342 for product-pilot economics and PR #350 for promotion experiments; avoid duplicate metric stores.

**Decision gate:** a live staging success with complete evidence enables a bounded pilot, not a production-readiness certification. Keep autonomous release off while a mandatory safety invariant fails or required evidence is missing. Defer expanded frameworks, paid fault-injection platforms and broad mutation suites until the targeted gaps are closed.

**Checked:** pinned status, drill, Makefile, CI workflow, scenario/claim test inventory, polygon ADR and task listing, Anthropic and Google primary references. **Inferred:** proposed gate sequence and priorities. **Confidence:** high for inspected test scope, medium for reported operational state. **Not verified:** local commands, CI outcomes, live credentials, current Mac stack, actual task execution or real deployment. DORA guide fetch failed; no new claim is based on that source.
