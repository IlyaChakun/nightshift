# Factory experimentation and controlled improvement

Make the existing self-improvement loop experimentally accountable. Compare candidate changes with a pinned baseline on independent, representative tasks; promote only improvements that satisfy quality, safety, cost and operator-attention constraints.

**Priority:** P2. **Status:** research proposal, not approved implementation.

## Contents

- [Problem](#problem)
- [How it works](#how-it-works)
- [Trade-offs](#trade-offs)
- [Sources](#sources)
- [Fit](#fit)
- [Status](#status)
- [Proposal](#proposal)
- [Acceptance and stop conditions](#acceptance-and-stop-conditions)
- [Agent follow-up](#agent-follow-up)
- [Record and evidence](#record-and-evidence)
- [Decision brief — reproducible experiments before automatic improvement, 2026-11-26](#decision-brief--reproducible-experiments-before-automatic-improvement-2026-11-26)

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

## Decision brief — reproducible experiments before automatic improvement, 2026-11-26

**Decision:** improve the current local polygon rather than add another evaluation platform. Close manifest, run-history, coverage and independent grader gaps before automatic promotion. Phase 7 remains the implementation home; A02 governs protected promotion, A06 evidence integrity, A07 operational validation and #342/#345 outcome accounting.

### Pinned implementation findings

| Evidence | Disposition |
| --- | --- |
| [ADR 0011](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/adr/0011-polygon-runs-locally.md#L18-L33), [Runner](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/evals/run.go#L105-L175) | Throwaway local Git repository and code grader already work in the polygon. Hidden files copied after harness returns, symlinks removed. Useful fixture separation, not proof of security isolation: same host may expose task source, lingering processes or candidate-modified grading environment. Public fixture tests are regression cases, not secret holdouts. |
| [cacheKey](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/evals/run.go#L253-L274), [eval schema](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/schemas/eval-result.json#L38), [CLI](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/cmd/nightshift/evalrun.go#L34-L102) | Key hashes task directory plus candidate string. CLI candidate defaults to harness name; no automatic immutable model/config/evaluator identity in that key. Schema description promises broader SHAs. It is safe only if a caller supplies a complete identity, which the default does not guarantee. |
| [Result](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/evals/run.go#L76-L89), [attempt initialization](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/evals/run.go#L120-L125), [saveResult](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/cmd/nightshift/evalrun.go#L147-L159) | Attempt hard-coded to 1 and result path candidate/task.json; repeated same-name runs overwrite history. No confidence estimate from the last result alone. |
| [Runner errors](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/evals/run.go#L139-L155), [runSet](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/cmd/nightshift/evalrun.go#L162-L186) | Login/limit/error abort rather than save a normal result; Runner's comment above Run says crash counts against candidate, but implementation distinguishes outage. Preserve unrun/partial attempts with reason, resource use and independent outage classification instead of treating missing data as either pass or intrinsic defect. |
| [selection.yaml](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/evals/selection.yaml#L1-L28), [selector](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/evals/select.go#L55-L83) | Existing deterministic path rules. Skills trigger Builder/Verifier, not Lead, even for Lead-specific changes. Unmatched path triggers no eval; role and harness changes have limited levels/sets. This is a scheduling heuristic, not promotion coverage. Audit actual caller/gates before claiming missed production checks. |
| [CLI role handling](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/cmd/nightshift/evalrun.go#L72-L94), [Lead E1](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/cmd/nightshift/evalrunlead.go#L54-L90), [Lead case](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/cmd/nightshift/evalrunlead.go#L168-L197) | Builder and Lead paths differ, Lead loads role and skills; builder runner request in run.go carries role name, task prompt, output schema, environment, not the same full candidate context as actual worker. Verifier is not an accepted eval-run role in this CLI. Do not call a skill/config comparison meaningful unless the intervention actually reaches the harness. |
| [Plan phase 7](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/plan.md#L141-L143), [design polygon](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/design.md#L693-L709) | At pinned base only greeting task in evals/tasks plus separate Lead cases; designed 10-task portfolio, protected third, E2/E3 and learning loop remain incomplete. Existing deterministic tests and live checks are different evidence layers. |

### Executed mechanism check

A disposable Go 1.23.2 linux/amd64 program reproduced cacheKey's SHA256 algorithm (candidate + NUL, sorted fs.WalkDir paths, NUL-separated length and file bytes). With task/task.yaml containing id: example plus newline and candidate claude, two simulated external model configurations A and B returned the same key:

a75cb8c93173a62ee6ef9b18b95471d06c0fe7256dd70bb0f687c628aa064e60

Changing task content to id: changed produced b0b6f5a80193564605b0f720886da2ca41ec1b4b29ad330bfe3e38134e25d998. Writing two results with attempt 1 and different passed values to the same results/example.json left only the second. The fixture uses exact relevant hashing/write behavior, not the actual CLI or live models. It establishes identity/history limitations, not that a real cache hit or promotion bypass occurred. Add integration tests that change one manifest input at a time, and verify two actual CLI runs retain independent trials.

### Primary research (2026-11-26)

- [Anthropic, Demystifying evals for AI agents](https://www.anthropic.com/research/demystifying-evals-for-ai-agents), opened: evaluate actual environment outcome as well as trace, use code graders where suitable, calibrate model graders and account for stochastic attempts. Distinguishes pass@k (at least one succeeds) from pass^k (all succeed). Neither is a substitute for independent release evidence.
- [OpenAI evaluation best practices](https://developers.openai.com/api/docs/guides/evaluation-best-practices), search excerpt: task-specific tests and human-calibrated evaluation. Treat as guidance, not a commitment to a hosted platform; current documentation also signals deprecation of its Evals platform. Preserve portable local result format.
- [Microsoft Research, Diagnosing Sample Ratio Mismatch](https://www.microsoft.com/en-us/research/articles/diagnosing-sample-ratio-mismatch-in-a-b-testing/), opened: assignment/data integrity before interpreting effects. Transfer missing-run accounting and predeclared design, not web-experiment assumptions to paired coding trials.
- Existing #342 provides pilot design and uncertainty; #339 industry reports are first-party cases, not measured Nightshift gains. No external results establish our promotion threshold.

### Minimal experimental contract

Freeze a trusted experiment manifest before runs: ID, hypothesis, target failure class, authorized baseline/candidate commits and artifact digests, allowed difference, actual role/skill/rubric/stack/toolchain/harness/model/provider/effort versions, task dataset and split digests, grader and policy identities, environment/network permissions, seed if supported, per-trial and total budget, assignment/order, effect threshold, quality/safety floors, stop rules and owner. Never put secrets into manifest or hash inputs. Record actual resolved model and deviations, not just requested alias. Key content-addressed by immutable inputs; separate unique run/trial ID, timestamps and attempt identity from any reusable deterministic cache identity. Do not treat stochastic cached output as an independent trial.

Retain append-only per-trial results and artifacts for PASS, FAIL, NOT_RUN, INFRA_ERROR, TIMEOUT, CANCELED and UNKNOWN; include missing planned trials, actual exposure, raw grader outputs/uncertainty, usage provenance and stop reason. An unexpected abort is evidence about execution reliability but may not measure model capability; predeclare what belongs in each analysis, report both intention-to-evaluate and eligible completed analysis with missingness. Use atomic writes and no silent overwrite; keep retention/access policy. A run whose proposed intervention was not actually loaded is INVALID, not a negative efficacy result.

Use three datasets: development/target failures visible to learner, regression/capability cases with frozen criteria, and genuinely protected holdout owned by a trusted evaluator outside candidate access (not just hidden/ in public checkout). Split related variants by task family/repo to avoid leakage; preserve origin, versions, contamination status and licensing. Do not repeatedly tune to per-case holdout feedback. Limit exposure, report only appropriate aggregate decision signals, rotate under trusted owner control. If no protected holdout exists, label the result accordingly; never claim holdout generalization.

Trusted evaluator resolves selection from approved baseline policy and changed paths, supplements with mandatory suites for critical changes, records actual executed vs required tests and reasons. Use separate sandbox/process identity and approved credentials, terminate candidate processes before grading; stage candidate artifact into clean grader-owned environment; evaluator code, tests, policy, result store and promotion authority not writable by candidate. Add negative tests for accessible task source, altered grader/inputs, lingering process and forged result; do not assume symlink cleanup provides this boundary. Use A04 existing isolation improvements first.

### Experiment ladder and decision protocol

1. Add one failing real example as a frozen regression and one neighboring valid case. Reproduce baseline failure under actual configuration; verify candidate change reaches the harness or deterministic check.
2. Compare baseline/candidate on paired identical starting tasks with separate workspaces, randomized order, fixed model/config except the intervention, identical limits and independently frozen graders. Include failed/canceled/outage trials and costs. Repeat stochastic cases within owner-approved budget. Estimate per-task differences and uncertainty at task/family level, not independent-token or pooled-trial pseudo-replication.
3. Report first-try success, bounded eventual success and consistency separately. For an illustrative independent Bernoulli trial with probability p, at-least-one of k is 1-(1-p)^k and all k is p^k; actual correlated runs need empirical results. Avoid comparing different retry budgets or cherry-picking best-of-k. Track required regressions, escapes, true/false positives, accepted outcomes, cost and human effort.
4. Protect safety floors: any planted authorization violation or mandatory evidence bypass blocks promotion; a small sample of zero failures is not safety certification. Candidate improvement needs targeted regression fixed and no unacceptable regression on capability/holdout. Freeze effect threshold before confirmatory trials; underpowered/inconsistent result remains inconclusive. Retain negative results and cost. An LLM grader may return UNKNOWN; calibrate against independently labeled examples and deterministic behavior.
5. Independent authority reviews manifest, immutable results and actual candidate digest. Trial rules warn only where design allows; monitor exposure, false positives, quality and costs against baseline. Owner controls active rule weakening and protected changes. Promotion is not the candidate's decision. For stable factory upgrades use existing external promoter and known-good rollback (A02); canary later when actual operational recovery is proved.

### Acceptance and stop conditions

- Same candidate label with changed model/skill/grader/policy produces distinct manifest digest; same immutable inputs reproduce digest, different trial IDs never overwrite each other. Budget/rate-limit failures and missing assignments remain visible.
- Change a Lead skill and a critical selection rule; trusted evaluator includes correct required evals, and actual candidate configuration is loaded. Unmatched/unsupported required suite is not green. Deliberately degraded candidate with improved target case is rejected.
- Protected holdout/evaluator cannot be read or mutated by candidate or its lingering process; forged results, modified gates and stale artifact cannot promote. Test in an isolated environment, not on production.
- Paired comparison retains all planned trials and reports uncertainty; invalid evaluation cannot be used as a winner. Trial promotion and rollback are independently authorized and recorded.
- Start with one real defect, one intervention, small budget and existing polygon. Expand task portfolio and repeated trials only when needed to decide meaningful effects. Stop building infrastructure if the lightweight manifest/history plus independent runner suffices.

**Checked:** pinned eval runner, key/schema, CLI, selection rules, Lead path, ADRs/plan/design, actual local Go fixture, opened primary pages and search excerpt as labeled. **Inferred:** coverage, isolation and promotion risks, proposed experiment contract. **Confidence:** high for inspected key/history semantics; medium for full call-site enforcement. **Not verified:** actual model comparisons, protected holdout isolation, billing, full repo integration tests, live promotion or CI. No runtime change authorized here.
