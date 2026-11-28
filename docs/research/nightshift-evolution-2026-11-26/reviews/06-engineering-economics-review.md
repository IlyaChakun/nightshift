# Engineering economics and adaptive execution review (PR #345)

Keep the configured router while measuring the cost of an independently accepted task. The proposal correctly counts failed work, but today's job totals cannot distinguish all attempts or tell the owner what they paid. Start with reconciliation on one provider before comparing Builder settings.

Reviewed source `2edf7b6` against `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition | Evidence |
| --- | --- | --- |
| Job totals omit per-attempt history | Confirmed limitation. Reclaim increments one row's attempt count and preserves its first start. Completion stores one usage object. | `internal/book/queries/jobs.sql:23`, `internal/book/queries/jobs.sql:52` |
| Zero can mean free, unreported or unpriced | Confirmed. Fallback pricing uses input/output tokens and the final numeric field carries no provenance. Claude's stream excludes cache tokens. Codex sums turn usage. | `internal/runner/runner.go:211`, `internal/jobsapi/api.go:78`, `internal/jobsapi/api.go:138`, `internal/harness/claude/claude.go:349`, `internal/harness/codex/codex.go:247` |
| Timeline and routing still await live checks | Partly outdated. S2 timeline and S3 model selection have recorded live checks. S1 has a Claude run and an outstanding Codex check. The timeline hides zero cost and counts elapsed time across attempts. | `docs/plan/phase-5.md:29`, `internal/book/migrations/00016_dash_task_timeline.sql:21` |
| Budget controls and task counts serve different purposes from billing and objective acceptance | Unverified for aggregate charges. Routing handles availability and fallback. The supervisor evaluates streamed usage, while the end check considers tokens. Summary counts terminal task events. | `internal/dispatch/router.go:90`, `internal/harness/supervise/supervise.go:190`, `internal/book/queries/summary.sql:5` |

## Suggested decisions

- **Take, phase 5 S1 and limits stream.** Compare synthetic success, interruption and reclaim records with available provider totals. Key observations by job and attempt, retain missing usage, reported or estimated cost, rate-card version and residuals. Validate concurrent budget accounting before claiming an aggregate cap. Consider pre-dispatch reservations only if a hard cap is required.
- **Take, phase 7.** Pilot a read-only cohort report in the book. Include all assigned work and independent acceptances. Separate cash, allocated subscription/infra, notional cost and volunteered operator minutes. Report coverage, elapsed time and uncertainty. With zero acceptances leave the ratio undefined.
- **Later, phase 7.** Pair frozen tasks under two fixed Builder policies with the same acceptance criteria, budget and Lead/Verifier settings. Require repeatable gains before shadowing a reversible task-class rule. A learned router needs evidence that a simpler rule leaves meaningful savings.
- **Reject, phases 5/7.** A second gateway, an immediately adaptive router or agent-controlled spending would add complexity before cost or quality can be compared.

FinOps describes “tying technology costs more directly to business outcomes.” RouteLLM reports “Evaluations on public benchmarks,” a different workload from accepted coding tasks. Both sources were read 2026-11-26. [FinOps](https://www.finops.org/framework/capabilities/unit-economics/) · [RouteLLM](https://proceedings.iclr.cc/paper_files/paper/2025/hash/5503a7c69d48a2f86fc00b3dc09de686-Abstract-Conference.html)

**Checked** a temporary Go test against the pinned pricing implementation returned “known zero and missing price both yield zero.” Also read job SQL, runner, harnesses, supervisor, router and phase ledger. **Inferred** accounting priorities. **Unverified** invoices, subscription utilization, aggregate enforcement and routing savings. **Confidence** high for representation, medium for pilot benefit.
