# Factory experimentation and controlled improvement

Make the existing self-improvement loop experimentally accountable. Compare candidate changes with a pinned baseline on independent, representative tasks; promote only improvements that satisfy quality, safety, cost and operator-attention constraints.

**Priority:** P2. **Status:** research proposal, not approved implementation.

## Problem

A factory can modify its prompts, skills, standards and execution policies without becoming more effective. A candidate may improve familiar examples while degrading unseen tasks, consume more resources, exploit an evaluator or benefit from a model update unrelated to the change.

## How it works

Use the existing polygon, findings and fix ladder as the evaluation infrastructure. A proposed experiment record states the hypothesis, target failure class, baseline and candidate revisions, allowed changes, task selection, metrics, minimum useful effect, non-regression constraints, budget, owner and promotion rule. Freeze inputs, model/harness configuration and evaluator versions before running.

Use paired evaluations with independent workspaces and identical constraints where feasible. Report per-task outcomes, failures, cost, human time and uncertainty; repeat stochastic cases under the approved budget. Keep development cases separate from protected holdouts. A release decision must reference independent evidence, not a score produced solely by the candidate.

## Trade-offs

| Option | Benefit | Limitation |
| --- | --- | --- |
| Regression-only fix validation | Cheap and focused | Limited evidence of generalization |
| Paired baseline/candidate experiments | Measures incremental value | Evaluation cost and variance |
| Shadow or canary evaluation | Realistic workload evidence | Operational complexity, selection bias |
| Automated unrestricted self-modification | Rapid exploration | Untrusted authority and uncontrolled regressions; reject |

Begin with offline paired experiments using existing eval infrastructure. Add canaries only after live operations and rollback are proven. A candidate never controls the evaluator, holdout, promotion policy or its own authorization.

## Sources

- Design sections 10 and 20, ADR 0011 and ADR 0016 describe the polygon, rule lifecycle and learning signals.
- [DORA software delivery performance metrics](https://dora.dev/guides/dora-metrics/), search excerpt checked 2026-11-26: balance throughput with instability when assessing effects.
- Prior research PRs #333, #339, #341 and #342: trusted upgrades, industry cases, autonomy vision and product-pilot baseline. Review canonical sources before adopting any external technique.

## Fit

Nightshift already specifies trial rules, regression tasks, protected holdouts, stable instance upgrades and the fix ladder. The proposal adds an experiment protocol and evidence record, not a competing learning engine. Preserve the tagged stable factory as an independent operational baseline.

## Status

Partial foundation: design and plan document evaluation and clustering; phase 7 remains the home of self-improvement. Statistically meaningful candidate comparison and safe production promotion are not proven by the current status.

## Proposal

Use one observed failure from a real pilot. Select a single intervention, such as a more focused context brief, a skill revision or a deterministic rule. Freeze baseline/candidate identity and define metrics before seeing the results. Run the original failing cases plus regression and holdout cohorts. Record the decision even when the hypothesis fails.

Track independently accepted outcomes, targeted failure rate, false positives, safety failures, time and cost per accepted outcome, and human minutes. Use a hard safety floor; do not hide quality regressions behind a composite score. For sparse data, report inconclusive rather than choosing a winner. Separate development from holdout access and prevent iterative tuning against holdout feedback.

Promotion sequence: propose, evaluate, independent approval where required, bounded trial, monitor, promote or rollback. Owner-approved authority and spending caps remain fixed throughout. A model or provider version change requires a new comparison or an explicit comparability caveat.

## Acceptance and stop conditions

- An experiment is reproducible from pinned inputs, versions, results and recorded constraints.
- A deliberately degraded candidate is rejected even if its target example improves.
- Candidate jobs cannot read hidden tests or modify their promotion conditions.
- Inconclusive or negative experiments are retained and do not trigger automatic rollout.
- A promoted improvement has a monitored window and a verified rollback path.
- Stop expanding the framework if the existing polygon plus a lightweight experiment record is sufficient.

## Agent follow-up

Coordinate with PRs #333, #337, #341, #342 and #345. Validate the existing polygon and its access boundary before implementing experiment storage or adaptive policy. Use phase 7 rather than adding a new autonomous agent role.

## Record and evidence

Preserve this dated owner-requested proposal. Link later experiments, decisions and implementation PRs; do not silently delete or replace the source record. The living design remains `docs/design.md` and the canonical sources index is `docs/prior-art.md`.

**Checked:** [base 9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; design sections 10, 11, 12, 14, 20, 22, plan, status, research index and open PR titles. **Inferred:** candidate gaps, priorities and experiment design. **Confidence:** medium; no live validation or full code audit. External links identified above were checked only to the extent stated.
