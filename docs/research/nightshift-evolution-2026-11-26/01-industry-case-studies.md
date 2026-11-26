# Industry evidence: autonomous software factories

**Research snapshot:** 2026-11-26. **Purpose:** identify transferable patterns, not reproduce another company's entire architecture. **Evidence:** primary engineering publications from the companies themselves; their reported results are not independent benchmarks. The six primary publications below were reopened for this research. Nightshift-specific mappings refer to the earlier external audit, not a fresh inspection of HEAD.

## Executive conclusion

The industrial evidence supports bounded, increasingly autonomous software delivery when the environment is legible, workflows are durable, verification is independent and authority is constrained. It does not establish that unrestricted agents can reliably replace an engineering organization or discover commercially successful products by themselves. PRs, lines of code and test counts are activity indicators, not proof of quality or customer value.

## 1. Stripe — Minions

**Evidence:** Stripe reports more than 1,300 fully agent-produced PRs merged weekly as of 19 February 2026, still subject to human review. The number describes merged work, not autonomous success rate across all attempts.

**Pattern:** unattended coding in isolated, ready-to-use development environments; predictable deterministic workflow steps around flexible agent execution; access to organizational tools and context.

**Nightshift experiment:** compare clean per-job workspaces and a standardized build/test/repair path with current harness behavior. Make PR delivery a bounded, reusable workflow; verify credentials are task-scoped and environments cannot access other runs. Measure accepted tasks, intervention and startup overhead, not just PR volume.

**Trade-off:** prewarmed environments can save time but incur capacity and isolation costs. Human review in Stripe's setup cannot be treated as proof that unattended release is safe.

**Sources:** [Part 1](https://stripe.dev/blog/minions-stripes-one-shot-end-to-end-coding-agents), [Part 2, 2026-02-19](https://stripe.dev/blog/minions-stripes-one-shot-end-to-end-coding-agents-part-2).

## 2. OpenAI — Harness Engineering

**Evidence:** the 11 February 2026 report describes a five-month internal product experiment, approximately one million repository lines (including infrastructure, documentation and tooling), about 1,500 merged PRs, hundreds of internal users, and an engineering team that grew from three to seven. The reported absence of hand-written code does not mean humans were absent: they specified intent and engineered the environment.

**Pattern:** make repository knowledge navigable, expose application behavior and telemetry to agents, enforce architectural constraints mechanically, and invest in tools whenever the same failure recurs.

**Nightshift experiment:** maintain a short navigation index for agents, structured knowledge with ownership/freshness, and independently verified per-worktree previews. Convert recurring failure classes into reusable tests or tools. Compare a task cohort before and after the improvement.

**Trade-off:** harness investment consumes engineering time; enormous context files create noise; automated cleanup must not silently change requirements. The experiment does not prove multi-year maintainability.

**Source:** [Harness engineering, 2026-02-11](https://openai.com/index/harness-engineering/).

## 3. OpenAI — Performance Factory

**Evidence:** the 19 November 2026 report attributes more than 1,700 investigations and 600 root-caused cases to its regression workflow in just over three months, approximately 17 investigations a day. Engineers review proposed fixes; after deployment an agent verifies recovery.

**Pattern:** explicit configurable task states, dispatch, cloud execution, subscriptions to external changes, adversarial diagnosis review, and production feedback. Prior regressions become evaluation inputs; recurring problems become self-repair opportunities.

**Nightshift experiment:** add a narrow tenant-authorized incident workflow: alert → deduplicated investigation → evidence-backed diagnosis → independent review → bounded fix → approved deployment → actual recovery check. Use existing dispatcher, durable state and integration paths where possible. Make alerts actionable, not a source of uncontrolled ticket creation.

**Trade-off:** production telemetry may contain sensitive data; read-only, tenant-scoped access and evidence retention matter. Investigation volume is not number of deployed fixes or prevented incidents.

**Source:** [Performance Factory, 2026-11-19](https://openai.com/index/how-openai-performance-factory-keeps-chatgpt-fast/).

## 4. StrongDM — Software Factory / Attractor

**Evidence:** the 6 February 2026 account describes non-interactive implementation driven by specifications and scenarios, with no traditional human-written code or human code review. Public evidence does not provide a comparable long-run defect or cost-per-feature rate.

**Pattern:** external behavioral acceptance, digital twins of dependencies, shift from interactive discovery to specified unattended work, reference implementations, file-based knowledge, and progressive summaries. Attractor is a useful reference for the boundaries between orchestration and coding-agent runtime.

**Nightshift experiment:** independently authored holdout scenarios, a small twin for the pilot's most important dependency, a versioned scenario digest, and cross-task product acceptance. Deliberately introduce a faulty implementation that passes visible tests; verify that the holdout detects it.

**Trade-off:** twins can drift from actual services, and external behavior alone may miss security, operability or architecture defects. Keep contract parity checks and explicit non-functional gates. Do not copy the company's spending heuristic as a budget target.

**Sources:** [Software Factory](https://factory.strongdm.ai/), [Techniques](https://factory.strongdm.ai/techniques), [Attractor reference — link to recheck](https://github.com/strongdm/attractor).

## 5. Ona — Memo experiment

**Evidence:** on 30 April 2026 Ona reported a ten-day experiment building Memo, a note-taking product, with 375 merged PRs, 67,000 lines, 1,067 tests and roughly 87% of merged work without human involvement. Humans provided specs, configured automation and handled escalations. Reported median issue-to-close and PR-to-merge times were 38 and 4.9 minutes respectively.

**Pattern:** demonstrate the entire lifecycle on a concrete product rather than optimizing the factory in isolation; use event-triggered automations for development, operations and iteration.

**Nightshift experiment:** run the product pilot in [04-product-pilot-and-measurement.md](04-product-pilot-and-measurement.md); publish a run journal and record every unplanned intervention. Compare to the same coding agent without the factory.

**Trade-off:** short demonstration on one application does not establish sustained adoption, security or total cost advantage; PR and test volume may reward fragmentation.

**Sources:** [Ten-day experiment, 2026-04-30](https://ona.com/stories/software-factory-what-we-learned), [public dashboard — link to recheck](https://software-factory.dev/).

## 6. Uber — Efficiency at scale

**Evidence:** the 27 August 2026 report attributes more than 70% of PRs to local or cloud agents, reports over 3,600 skills and more than 30,000 skill executions daily. It describes growing agent use while aggregate AI spend had relatively stabilized since April. These figures include human-initiated and assisted work, not purely autonomous delivery.

**Pattern:** routing, reusable skills, context and cache optimization, shared tool access, and evaluation of model choices against real engineering tasks.

**Nightshift experiment:** instrument tokens, cost and latency by model, workflow phase, tenant and outcome. Use a small frozen task set to compare a cheaper route against a stronger baseline. Promote routing changes only with quality non-regression and reliable cost accounting.

**Trade-off:** routing policies, caching and gateways add complexity; premature optimization can hurt success rates. Never let a cheaper route bypass security or acceptance. Start with cost attribution and a simple routing policy.

**Source:** [Running a Software Factory Efficiently at Uber Scale, 2026-08-27](https://www.uber.com/us/en/blog/efficient-software-factory/).

## Counterevidence and further reading

- [SWE-Marathon](https://www.swemarathon.org/): long-horizon software tasks; verify the paper version, task distribution, models and scoring before citing numerical results. The landing page was not readable during this refresh.
- [Writing Code vs. Shipping Code, NBER working paper w35275](https://www.nber.org/papers/w35275): investigate the difference between code output and shipped outcomes. The full page returned 403 during this refresh; no numerical claim is made here.
- [Nightshift prior art](https://github.com/IlyaChakun/nightshift/blob/main/docs/prior-art.md): reconcile these cases with existing research before adding duplicated records. GitHub fetch failed during this refresh.

## Transfer matrix (proposals, not implementation claims)

| Pattern | Primary inspiration | Candidate Nightshift owner / surface | Proof of usefulness |
|---|---|---|---|
| Isolated reproducible run | Stripe | Worker / harness | Cross-job access denied; startup and success measured |
| Repository knowledge and tools | OpenAI Harness | Lead / harness / docs | Repeated failure reduced on holdout |
| Event-driven operational remediation | OpenAI Performance | Dispatcher / outbox / telemetry | Verified recovery after actual staged regression |
| Hidden behavioral verification | StrongDM | Independent Verifier / release gate | Faulty candidate rejected; correct one accepted |
| Real product lifecycle | Ona | Tenant / product owner | Verified deployed outcomes, real usage, intervention log |
| Cost-aware routing | Uber | Harness / budgets / observability | Lower cost per accepted unit without degraded quality |

## Instructions for research agents

Reopen sources, store publication dates and claim-level links, and mark each technique as implemented, partial, missing, not applicable or unverified against a pinned Nightshift commit. Map to existing ADRs and audit findings. For each proposed change define baseline, expected outcome, simplest experiment, guardrails and rollback. Add evidence to the existing research index instead of creating an unrelated second knowledge base. Do not treat corporate self-reports as independently verified benchmarks or copy organizational scale assumptions into a small deployment.

## Repository reconciliation and retention

This dated external research proposal is not a normative design change. Preserve the snapshot and append links to subsequent decisions and evaluation evidence rather than deleting it when superseded. Existing `docs/prior-art.md` is the canonical systems index; `docs/research/README.md` describes techniques. This snapshot is evidence and proposed experiments, not a second living knowledge base. Reconcile each case with the canonical index and link rather than duplicate durable facts.

**Checked:** repository rules, status and research index at [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26. External source dates and links are listed in the research pack. **Inferred:** proposed architecture, experiments and maturity targets; no implementation or runtime validation. **Confidence:** medium.
