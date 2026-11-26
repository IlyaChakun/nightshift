# Production intelligence and incident recovery review (PR #346)

Use one synthetic staging incident to distinguish a completed rollback from a recovered service. The existing health worker already opens repository maintenance tasks, so a new incident intake system would duplicate some of its work. The proposal's strongest addition is an independent observation of the original user-visible failure after recovery.

Reviewed source `03d9f62` against `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition | Evidence |
| --- | --- | --- |
| PR #328 is pending | Already implemented. The hourly worker caps new tasks at three, deduplicates across task states and considers canceling queued work when the signal clears. It handles repository alerts and idle PRs. | `internal/dispatch/healthtasks.go:30`, `internal/dispatch/healthtasks.go:83`, `internal/dispatch/healthtasks.go:118`, merge `8b9f966` |
| Support and tracing are usable foundations | Confirmed. Support requires reproduction evidence or a visible uncertainty statement. A task trace derives from tenant, task and creation time. Product symptoms need separate service/revision evidence. | `skills/own/support/SKILL.md:18`, `internal/dispatch/tracing.go:17`, `internal/summary/health.go:35` |
| Deploy journals smoke and rollback, and Compose can run a project smoke | Confirmed. Compose records a previous revision when it reads a healthy stack. The journal records the rollback result. A separately observed product-recovery window remains unverified. | `internal/adapters/deploy/composelocal/composelocal.go:271`, `internal/adapters/deploy/composelocal/composelocal.go:293`, `internal/deploy/deploy.go:207` |
| Tenant policy for a required smoke check | Partly. The adapter contract includes optional capabilities. Whether a given tenant requires independent behavior evidence needs its effective project policy. | `docs/design.md:1263`, `docs/plan/phase-6.md:5` |
| Phase 6 has never run on a real engine | Already superseded. The first real-engine check led to a rollout fix and a recipe change. Their live rerun and P5 acceptance remain open. | `docs/plan/phase-6.md:10`, `internal/deploy/deploy_test.go:137` |

## Suggested decisions

- **Take, phase 6 P5/P6.** Use one isolated service, a reversible regression, a manual signal and the support skill. Before starting, choose the failing user behavior, sample minimum, observation window and revision. Have an independent reviewer challenge the diagnosis. Recheck service behavior and revision after an authorized repair or rollback. Record unavailable checks and partial recovery explicitly.
- **Take, phase 6.** Use the existing task to record delivery ID, condition and occurrence, tenant/service, redacted evidence, linked release, investigation budget and escalation. Exercise repeats, recurrence, missing observations and restart. Compare manual and assisted triage by false diagnoses, verified recoveries and active operator time, including unresolved cases.
- **Later, phase 6 after the pilot.** Add an authenticated signal adapter and a durable incident lifecycle when multiple signals/tasks demand them. Defer automatic mitigation until a reversible action, cooldown, independent probe, kill switch and owner policy are demonstrated.
- **Reject, phase 6.** A general production shell, new SRE role or additional monitoring service gives the pilot more authority and infrastructure than it needs.

Alertmanager describes “deduplicating, grouping, and routing” alerts. OpenAI reports “a manual review of 100 root cause suggestions,” which supports challenging diagnoses without predicting Nightshift accuracy. Read 2026-11-26. [Prometheus](https://prometheus.io/docs/alerting/latest/alertmanager/) · [OpenAI](https://openai.com/index/how-openai-performance-factory-keeps-chatgpt-fast/)

**Checked** health-task cap and rollback tests passed on synthetic data and twins. Read health and deploy paths, support skill and phase ledger. **Inferred** staging pilot scope. **Unverified** live recovery, incident deduplication across conductors and diagnosis quality. **Confidence** high for implementation status, medium for pilot benefit.
