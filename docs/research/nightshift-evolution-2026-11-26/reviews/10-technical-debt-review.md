# Technical debt and architecture checks review (PR #349)

Choose one recurring cost and compare deferral, a small repair and a preventive check. The existing finding history can identify candidates, but a finding marked fixed when Builder receives it cannot establish that maintenance work succeeded. Cost estimates and a fall in finding counts both need independent evidence before they justify a refactor.

Reviewed source `519df24` against `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition | Evidence |
| --- | --- | --- |
| Audit and clustering exist; rule promotion remains open | Partly. The audit skill asks for measured results. Clustering counts distinct tasks by rule or category and words. Phase 7 still lists the weekly tenant report and rule-generation work as open. | `skills/own/audit/SKILL.md:17`, `internal/learn/cluster.go:44`, `internal/learn/cluster.go:60`, `docs/plan/phase-7.md:13` |
| Finding handoff can be mistaken for repair | Confirmed limitation. Builder enqueue marks its input findings fixed at handoff. Closure and recurrence require a subsequent observation of comparable work. | `internal/dispatch/build.go:276`, `internal/dispatch/build.go:290` |
| Active standards and override checks exist | Partly. The checklist selects active standards; resolution enforces precedence and override reasons. These operations do not measure a trial's false positives or authorize promotion. | `internal/standards/standards.go:40`, `internal/standards/cards.go:214`, design §§9–10 |
| The baseline example demonstrates a helper-level limitation | Confirmed by source inspection at helper level. The real runner applies the helper and carries tool failures separately. Its effect on a deployed policy is unverified. | `internal/check/run.go:108`, `cmd/nightshift/check.go:68`, `internal/check/sarif_test.go:52` |

## Suggested decisions

- **Take, phase 3/4.** Define and test how a reviewed historical exception behaves when rule version, severity or scope changes. Use an ordinary quality rule, an allowed neighboring change and an unavailable tool. Record revision, tool and policy provenance. Preserve mandatory checks when adding warn-only trials for optional rules.
- **Take, phase 7.** First distinguish handoff from verified closure. Link one recurring tenant/component finding to its invariant, observed consequence, owner, review window and decision. Compare deferring to a trigger, a behavior-preserving repair, and a native deterministic check. Record measured rework separately from estimated future burden.
- **Later, phase 7.** Build a debt view on the book once reliable evidence exists. Track exposure, adjudicated detections, exceptions, check maintenance and active human minutes. Promote, revise or retire a trial after independent review. Add persistent debt records only if accepted deferrals cannot be represented.
- **Reject, phase 7.** A blanket refactoring agent, universal complexity quotas, an undifferentiated debt score or a second quality service has no measured benefit here. An optional cleanup needs its own bounded task and behavior checks.

SEI describes “unexpected rework costs” and demonstrates its architecture metric on one development effort. This supports measuring a local consequence before adopting a numerical rule. Read 2026-11-26. [SEI architectural debt study](https://www.sei.cmu.edu/library/in-search-of-a-metric-for-managing-architectural-technical-debt/)

**Checked** existing baseline-count and recurring-cluster tests passed. Read finding handoff, standards, check-runner wiring and phase status. **Inferred** usefulness of a scoped debt decision. **Unverified** integrated severity-change outcome, production effects, trial promotion and measured maintenance savings. **Confidence** high for static findings, medium for projected value.
