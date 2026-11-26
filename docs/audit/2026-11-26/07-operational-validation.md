# NS-A07 — Operational validation and continuous improvement

**Priority:** P1 for live end-to-end and recovery proof; P2 for quality analytics and advanced evaluation.  
**Status:** validation and measurement roadmap.

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
