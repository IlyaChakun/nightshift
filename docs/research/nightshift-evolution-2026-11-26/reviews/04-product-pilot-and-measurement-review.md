# Product pilot and measurement review (PR #342)

Prepare one staged, owner-approved objective and a protected run journal before committing to a 30-day pilot. Count human minutes across every assigned objective, including failures, and divide by independently accepted objectives. If none is accepted, publish minutes and failure counts and leave the ratio undefined.

Checked main `8171d5ea85723b73185fd65acb27d4ded7134b82` against research head `ba15727`.

| Claim | Disposition | Evidence at checked main |
| --- | --- | --- |
| Existing book, trace and Grafana surfaces can support pilot accounting | Confirmed for tasks. The tenant-scoped timeline joins events and jobs. The dashboard view projects exits, times, tokens and cost. | `internal/summary/timeline.go:30`, `internal/dispatch/tracing.go:17`, `internal/book/migrations/00016_dash_task_timeline.sql:6` |
| Task records supply inputs to the proposed objective ledger | Partly. A requeued job can span several attempts. The dashboard suppresses zero-valued usage, so it cannot establish a measured zero spend. The proposed human and objective records need a separate source. | `internal/summary/timeline.go:98`, `internal/book/migrations/00016_dash_task_timeline.sql:23` |
| Live Lead and deployment evidence remains incomplete | Partly. The ledger reports five intake cases without a result file. A real deployment exercise informed fixes. Staged acceptance and a rerun remain open. | `docs/plan/live-checks.md:170`, `docs/plan/phase-6.md:10` |

## Decisions

- **Take — phases 5–6.** Preregister a bounded real-user problem, scope, stop criteria and non-sensitive staging setup. Pin model, base, policy, spec, evaluator and candidate. Complete applicable live safety, backup, budget and rollback checks before the affected operations. Record actual active minutes by category, attempts, approvals, rescue, maintenance, cost uncertainty and escaped defects. Stop on a mandatory control failure.
- **Take — phase 6.** Compare frozen paired objectives against the same standalone harness. Match resources and tools, randomize order, prevent answer transfer and preserve failures. Keep calibration separate, publish raw counts and limitations, and use one predeclared user outcome with a denominator and observation window. Review blockers daily, outcomes weekly and end with a proceed, adjust or stop decision.
- **Later — phases 6–7.** Expand into a longer product lifecycle, additional baseline, automated events, alerts and dashboards when manual measurement exposes a gap. Evaluate factory changes separately with independent holdouts and rollback. New spend and external access need owner approval.
- **Reject — all phases.** Fixed percentage, task, release and calendar targets as promotion gates encourage task splitting. Treat the suggested calibration size as an estimate pending variance and budget.

## Primary evidence

Read 2026-11-26. METR reports developers “take 19% longer” in its initial population. Cui et al. report a “26.08% increase” in completed tasks in other settings. Different endpoints support local measurement. [METR study](https://metr.org/blog/2025-07-10-early-2025-ai-experienced-os-dev-study/), [follow-up](https://metr.org/blog/2026-02-24-uplift-update/), [Cui et al.](https://www.microsoft.com/en-us/research/publication/the-effects-of-generative-ai-on-high-skilled-work-evidence-from-three-field-experiments-with-software-developers/).

DORA recommends measuring “one application or service at a time”. SPACE says productivity “cannot be measured by a single metric or dimension”. Both were read 2026-11-26. [DORA](https://dora.dev/guides/dora-metrics/), [SPACE](https://www.microsoft.com/en-us/research/publication/the-space-of-developer-productivity-theres-more-to-it-than-you-think/).

**Checked** Timeline, SQL view, phase 6 and live evidence. **Inferred** A manual pilot can expose the next measurement requirement. **Unverified** Pilot results, statistical power, actual spending, production readiness and customer benefit. **Confidence** High for accounting limitations, medium for feasibility.
