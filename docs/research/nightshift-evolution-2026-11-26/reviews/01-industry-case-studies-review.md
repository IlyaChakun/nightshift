# Industry case studies review (PR #339)

Use the real staging pilot to test whether Nightshift can independently accept the delivered change and account for the time people spend on it. The six company accounts offer specific experiments, but they use different units, from investigations to merged PRs. None provides a comparative effect size for Nightshift.

Checked main `8171d5ea85723b73185fd65acb27d4ded7134b82` against research head `f743e43`.

| Claim | Disposition | Evidence at checked main |
| --- | --- | --- |
| Workspace preparation and cleanup exist | Confirmed for the runner lifecycle. Concurrent isolation and reclaim need separate evidence. | `internal/runner/runner.go:158`, `docs/plan/live-checks.md:134` |
| Repository guidance, code checks and twins are foundations | Confirmed. Roles receive instructions and skills, and scripted scenarios have a runner. | `internal/runner/context.go:51`, `internal/runner/check.go:66`, `internal/dispatch/scenario_integration_test.go:27` |
| Live Lead and deployment still require proof | Partly. The ledger records five Lead intake cases without a retained result file. Deployment fixes await a real-engine rerun. | `docs/plan/live-checks.md:170`, `docs/plan/phase-6.md:10` |
| Cost, routing and operational feedback exist | Partly. Provider choice, task usage and deployment smoke exist. These measure different things from accepted-objective cost and incident recovery. | `internal/dispatch/router.go:90`, `internal/summary/timeline.go:53`, `internal/deploy/deploy.go:224` |

## Decisions

- **Take — phase 6.** Use one approved objective, a versioned independent acceptance scenario, the actual staged candidate, one deliberately incorrect outcome and a correct control. Record every attempt, worker startup, operator minutes and any mismatch between twin and staging. Reuse #342's ledger.
- **Take — phases 5 and 7.** Keep the existing repository map and canonical prior-art index. Link recurring navigation failures to tests or tools and assess them on held-out tasks. Measure startup before considering a warm pool.
- **Later — phases 6 and 7.** Test one deduplicated staging alert through observed recovery. Compare cheaper routing on fixed tasks only once total cost and independent acceptance can be measured. Try an impact brief with #344 if the pilot reveals missed consumers.
- **Reject — all phases.** A replacement factory, provider-specific gateways, company spending targets or PR-count goals would add cost without answering the pilot question.

## Primary evidence

Read 2026-11-26. Stripe describes “human-reviewed” work, OpenAI calls its setting an “internal beta”, and its performance team “graded 70% as accurate” in a manual review of 100 proposed causes. [Stripe](https://stripe.dev/blog/minions-stripes-one-shot-end-to-end-coding-agents-part-2), [OpenAI harness](https://openai.com/index/harness-engineering/), [OpenAI performance](https://openai.com/index/how-openai-performance-factory-keeps-chatgpt-fast/).

StrongDM describes “specs + scenarios”, Ona says “Humans wrote the specs”, and Uber counts PRs “attributed to local or cloud agents”. The denominators preclude a common autonomy rate. [StrongDM](https://factory.strongdm.ai/), [Ona](https://ona.com/stories/software-factory-what-we-learned), [Uber](https://www.uber.com/us/en/blog/efficient-software-factory/).

**Checked** Runner, dispatch, deployment, status and six primary accounts. **Inferred** Pilot priorities. **Unverified** Corporate outcome audits, historical supplement attachment, live isolation, staging acceptance and cost advantage. **Confidence** High for source scope, medium for transfer.
