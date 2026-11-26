# Factory experiment identity and trial history review

Fix how the local polygon identifies and retains trials before comparing candidates. A passing greeting task proves the runner can grade that fixture. It cannot estimate how consistently a skill improves other tasks.

Reviewed PR #350 against `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition on main |
| --- | --- |
| Experiment identity captures all configured inputs | The audit correctly identifies a mismatch. The schema describes model, skill, standard and harness identity. The key hashes the task tree and candidate string, which defaults to a harness name. `schemas/eval-result.json:38`, `internal/evals/run.go:254`, `cmd/nightshift/evalrun.go:65`. |
| Independent attempts survive reruns and outages | Confirmed limitation. The runner initializes attempt 1. The writer uses the same candidate/task filename and the loop returns on an error before a result is saved. `internal/evals/run.go:125`, `cmd/nightshift/evalrun.go:148`, `cmd/nightshift/evalrun.go:176`. |
| The polygon covers each affected role | Partly. Selection maps skill changes to Builder and Verifier. The run CLI supports Builder and Lead. Lead loads its role instructions and skills separately. `evals/selection.yaml:10`, `cmd/nightshift/evalrun.go:73`, `cmd/nightshift/evalrunlead.go:111`. |
| Representative protected holdout is already available | Unverified. The Builder task tree contains one greeting fixture. Its hidden test is tracked in the repository. The design's larger portfolio and paired comparison belong to phase 7. `evals/tasks/greeting/task.yaml:2`, `docs/design.md:1701`. |

**Local reproduction.** Two runs of `nightshift eval run --scenario testdata/evals/greeting.yaml --candidate same` with the same temporary output directory each reported `greeting pass`. The directory contained one `same/greeting.json`, with `attempt` equal to 1.

**Take, phase 7.** Give every trial its own result, including aborted or unrun trials. Record the effective inputs separately from the trial ID. Prove two identical invocations retain two outcomes and changing the actual role configuration changes the input identity. Reconcile selection against runnable roles before using a result to judge that role.

**Take, phase 7 after that.** Start with one observed failure, one neighboring valid task and one candidate change. Freeze the paired inputs, grader, budget, acceptance threshold and independent decision maker before evaluation. Record quality, operator time, cost and missing runs separately. Preserve an inconclusive result.

**Later, phase 7.** Expand held-out task families and repeated trials as independent evaluation and budget permit. Reserve canaries for a proven rollback path. **Reject** unrestricted self-promotion and a second eval platform.

“Each attempt at a task is a trial.” Read 2026-11-26. [Anthropic's evaluation terminology](https://www.anthropic.com/research/demystifying-evals-for-ai-agents)

**Checked** Builder and Lead CLI paths, selector, schema and fixture inventory. **Inferred** Experiment design. **Confidence** High for identity and file semantics. Model effects, holdout independence, promotion and operator cost remain unmeasured.
