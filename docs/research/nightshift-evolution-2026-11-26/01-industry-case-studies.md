# Industry evidence: autonomous software factories

**Research snapshot:** 2026-11-26. **Purpose:** identify transferable patterns, not reproduce another company's entire architecture. **Evidence:** primary engineering publications from the companies themselves; their reported results are not independent benchmarks. The six primary publications below were reopened for this research. Nightshift-specific mappings refer to the earlier external audit, not a fresh inspection of HEAD.

## Contents

- [Executive conclusion](#executive-conclusion)
- [1. Stripe — Minions](#1-stripe--minions)
- [2. OpenAI — Harness Engineering](#2-openai--harness-engineering)
- [3. OpenAI — Performance Factory](#3-openai--performance-factory)
- [4. StrongDM — Software Factory / Attractor](#4-strongdm--software-factory--attractor)
- [5. Ona — Memo experiment](#5-ona--memo-experiment)
- [6. Uber — Efficiency at scale](#6-uber--efficiency-at-scale)
- [Counterevidence and further reading](#counterevidence-and-further-reading)
- [Transfer matrix (proposals, not implementation claims)](#transfer-matrix-proposals-not-implementation-claims)
- [Instructions for research agents](#instructions-for-research-agents)
- [Repository reconciliation and retention](#repository-reconciliation-and-retention)
- [Deep research synthesis — 2026-11-26](#deep-research-synthesis--2026-11-26)

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

## Deep research synthesis — 2026-11-26

**Decision:** prioritize independent acceptance, predictable isolated execution and outcome measurement before adding more autonomous workflows. The six industrial cases above support these engineering patterns, not a universal claim of economic advantage or unattended production safety.

This supplement reconstructs the substantive findings from the completed Deep Research report visible in the conversation. Its separate Markdown attachment was not accessible to the committing session; this is a checked synthesis, not a byte-for-byte transfer of that attachment. Primary links below were reopened on 2026-11-26.

### Additional evidence and caveats

| Case | Evidence beyond activity counts | Transferable decision |
| --- | --- | --- |
| [Stripe Minions, part 2](https://stripe.dev/blog/minions-stripes-one-shot-end-to-end-coding-agents-part-2) | 1,300+ weekly merged agent PRs still receive human review; prewarmed isolated devboxes target short startup. | Measure cold/warm execution and enforce scoped access before pooling workers. Do not infer a safe automatic release rate. |
| [OpenAI Harness Engineering](https://openai.com/index/harness-engineering/) | The greenfield experiment emphasizes missing tools, machine-readable repository knowledge and mechanical architecture checks as bottlenecks. Its time-saving figure is an estimate, not a matched trial. | Make the repository legible first; turn recurring failures into tools and executable tests rather than ever-longer prompts. |
| [OpenAI Performance Factory](https://openai.com/index/how-openai-performance-factory-keeps-chatgpt-fast/) | Human assessment of 100 proposed root causes rated 70% accurate; the rest needed refinement or did not establish causality. Independent challenge and post-deployment recovery checks are integral. | Require evidence-backed diagnosis, disconfirming tests and observed recovery; a plausible explanation or merged PR is not closure. |
| [StrongDM techniques](https://factory.strongdm.ai/techniques) | Independent behavioral validation and replayable dependency twins substitute for traditional inspection in its operating model. | Validate a twin against its real contract; keep independent acceptance plus explicit security and operational gates. |
| [Ona ten-day experiment](https://ona.com/stories/software-factory-what-we-learned) | Early factory setup consumed a reported 8–10 human hours/day, declining to 2–3 in week two; ambiguous specs and visual quality remained problems. | Count harness maintenance and specification time, use visual references where relevant, connect user feedback to planning early. |
| [Uber efficiency](https://www.uber.com/us/en/blog/efficient-software-factory/) | Holding a model fixed helped distinguish efficiency improvements from model changes; workload mix and adoption still affect observed unit costs. | Compare routing and context changes on fixed tasks and model versions; measure cost per independently accepted objective, not requests alone. |

All six are first-party engineering accounts. Different populations, definitions, observation windows, and review practices prevent a credible numerical ranking of the factories. None establishes Nightshift's expected success rate.

### Nightshift applicability and ordering

The following mappings rely on the pinned base [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15) and prior audit inspection, not a fresh execution of the factory.

| Priority | Existing foundation and gap | Smallest useful experiment / pass criterion |
| --- | --- | --- |
| P0 | A01/A02/A04: hidden input filtering and release policy exist; independent execution and final-artifact evidence remain unproven. | A candidate that passes public tests but violates a required hidden criterion cannot merge/deploy under the applicable policy. A changed candidate invalidates dependent evidence. |
| P1 | A04: reusable worker, per-tenant HOME and cleanup; attempt-specific isolation not proven. | Concurrent synthetic jobs and expired-lease reclaim cannot read or alter protected peer inputs or accepted artifacts; record startup overhead. |
| P1 | A07 and pilot #342: deterministic tests, fake scenarios, reported local drills; live Lead/deployment still open at the pinned status. | One genuine product objective through actual integrations and verified staging release; preserve all failed attempts and operator minutes. |
| P1 | #344: repository instructions and role context exist; targeted impact intelligence remains a proposal. | Matched brownfield tasks with/without a revision-bound impact brief; measure misses, accepted outcomes and total cost. |
| P1–P2 | #346: durable dispatcher and observability foundation; closed-loop incident recovery remains a proposal. | Seed a staging regression, require independent diagnosis and verify recovery against the original signal. |
| P2 | #345/#350: budget and role model/effort configuration exist; comparative routing and promotion experiments remain proposals. | Freeze task, evaluator and model; test one intervention at a time; accept only measurable benefit above quality and safety floors. |

### Architectural decision and stop conditions

Reuse the Go control plane, Postgres book, existing harnesses and current release authority. Alternative A (incremental improvements) has the lowest migration and maintenance burden and is recommended. Alternative B (adopt a complete third-party factory) is unjustified without evidence that the existing substrate blocks a required capability. Alternative C (introduce specialized execution or evaluation components) is reasonable only when a specific isolation, fidelity or scale requirement cannot be met by the existing implementation.

Use the pilot to select the next one or two changes, not the largest number of industrial techniques. Stop adding machinery when it cannot demonstrate a measurable advantage on matched tasks, introduces unacceptable security/operational cost, or duplicates an existing capability. Reconcile canonical prior-art entries through the index PR rather than making this dated record a living design.

**Checked:** original #339 document and primary pages above on 2026-11-26; pinned repository status, prior audit sources and completed Deep Research report. **Inferred:** transfer priorities and experiment design. **Confidence:** high for the cited first-party statements, medium for transferability. **Not verified:** independent corporate outcome audits, full original supplement attachment, live Nightshift behavior or local checks.
