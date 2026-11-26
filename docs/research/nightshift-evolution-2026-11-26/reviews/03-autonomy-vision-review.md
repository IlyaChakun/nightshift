# Autonomy maturity review (PR #341)

Use the proposed levels to name the next demonstration for a particular tenant and environment. The next decision depends on a repeatable staging delivery, including independent acceptance and recovery. Keep the existing phase plan and owner authority as the sources for scheduling and permissions.

Checked main `8171d5ea85723b73185fd65acb27d4ded7134b82` against research head `6b67007`.

| Claim | Disposition | Evidence at checked main |
| --- | --- | --- |
| Durable dispatch, external side effects and bounded worker runs exist | Confirmed as implementation. River schedules the outbox, and the runner applies job supervision. | `internal/dispatch/river.go:61`, `internal/dispatch/outbox.go:84`, `internal/runner/runner.go:247` |
| Earlier drill, Builder jobs and one released task were reported | Confirmed as operator-reported. Status records all three observations. Live Lead intake also has a recorded result, but repeatable staged acceptance and rollback remain open. | `docs/status.md:11`, `docs/plan/live-checks.md:170`, `docs/plan/phase-6.md:11` |
| Dependencies exist and some child reviews lack overall criteria | Confirmed. Spec expansion creates sibling tasks and stores dependency links. The expanded task's review explicitly omits whole-spec criteria. An existing integration test expects this behavior. Aggregate acceptance requires a separate decision. | `internal/dispatch/deps.go:146`, `internal/dispatch/verify.go:167`, `internal/dispatch/lead_integration_test.go:599` |
| Self-improvement can select and safely promote a better factory | Partly. Recurring findings are clustered. Candidate evaluation, rejection and promotion require additional proof. | `internal/learn/cluster.go:60`, `docs/plan/phase-7.md:13` |

## Decisions

- **Take — phases 5 and 6, levels 1–2.** Use the existing live-check ledger. Track all eligible starts, independent outcomes, approvals, unplanned rescue, resource costs and repeated staging recovery. Record missing evidence explicitly. Tie each claim to a revision, run and owner-defined threshold.
- **Take — phases 5 and 6, level 3.** Try one compatible and one incompatible set of changes against an integrated objective with a versioned contract and evidence. Preserve dependencies and re-evaluate after requirements change. Assess user outcomes separately.
- **Later — phase 7, level 4.** Use one recurring failure, a frozen baseline, a holdout, independent promotion, a small canary and a tested rollback. Preserve the rejection of harmful candidates. Proceed when phase 6 meets its criterion.
- **Later — phase 10, level 5.** Trial advisory product discovery, incident feedback and a documented no-build decision before proposing portfolio allocation or automated experiments. Cross-tenant learning requires explicit authority.
- **Reject — all phases.** A parallel official roadmap, blanket permission by maturity level, automatic promotion, a replacement framework or specialist roles without a measured need add authority or maintenance without evidence.

## Primary evidence

Read 2026-11-26. Anthropic recommends “only increasing complexity when needed”. METR measures task difficulty using “human task duration”; its time horizon cannot certify Nightshift's deployment reliability. [Anthropic](https://www.anthropic.com/research/building-effective-agents), [METR](https://metr.org/time-horizons/).

**Checked** Dispatch, runner, verifier context, learning and live-check plans. **Inferred** Ordering of demonstrations. **Unverified** Live cohort, higher-level attainment, economic or product gains, and cross-tenant learning. **Confidence** High for implementation scope, medium for sequencing.
