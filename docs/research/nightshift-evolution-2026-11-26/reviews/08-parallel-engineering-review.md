# Parallel engineering and semantic conflicts review (PR #347)

Keep parallel builds and test the changes together before merging. In a disposable Git fixture, both branches passed alone, but the rebased pair failed the same check with an unchanged patch ID. That demonstrates a dependency between changes without establishing how a live Nightshift merge would behave.

Reviewed source `db0ed1f` against `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition | Evidence |
| --- | --- | --- |
| Dependencies and capacity already order work | Confirmed. The graph rejects invalid references and cycles. The ticker advances tasks one by one per tenant; jobs still execute separately. | `internal/dispatch/deps.go:41`, `internal/dispatch/conductor.go:255`, `internal/dispatch/tick.go:176` |
| The train already checks the exact combined candidate against the latest base | Partly. It journals a rebase, polls CI and asks the forge to merge an expected head. Effective host requirements and exclusivity throughout a pending CI run remain unverified. | `internal/train/train_test.go:110`, `internal/train/train_test.go:151`, `internal/adapters/github/github.go:248` |
| Patch identity cannot prove unchanged behavior | Confirmed limitation. The rebase compares patch IDs. In the local arithmetic fixture, each branch returned 0, Git rebased cleanly, and the combined test returned 1. | `internal/gitops/rebase.go:85`, disposable Git/Python run on 2026-11-26 |
| Two changes claimed migration 00015 | Confirmed by the retrospective and the actual renumbering commit. The source points at an older status page. | `docs/retro/2026-11-26-long-threads.md:51`, commit `acf8fd0` |

## Suggested decisions

- **Take, phase 3 X2.** Add combined-candidate cases for a producer/consumer pair, duplicate migration, shared application invariant and independent changes. Record base, integrated tree, checks, policy and approval identity. Measure when a conflict surfaces, rework, throughput and false overlap. Verify evidence applicability when the candidate changes.
- **Take, phases 3/6.** Validate one owner per target across CI, restart, cancellation and an external base update. Check effective host rules and reconcile the actual merged tree with the approved deployable artifact. Reuse the train and approval path. Add a Postgres ownership record only if tests show it is needed.
- **Later, phases 3/10.** Pilot advisory footprints from planned and actual changes when conflicts recur. Consider scoped reservations or speculative CI only when measured contention or queue delay pays for them. For a real multi-repository rollout, record compatible deployed versions, old/new combinations, ordered migration and a tested rollback.
- **Reject, phases 3/10.** Serializing every build, a second scheduler or distributed locking service would impose cost on independent changes. Separate repositories require explicit compatibility windows.

GitHub evaluates required checks against the “latest version of the target branch.” Git calls patch IDs “reasonably stable.” Neither source establishes Nightshift's effective branch rules. Read 2026-11-26. [GitHub merge queue](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue) · [Git patch-id](https://git-scm.com/docs/git-patch-id)

**Checked** clean-run and pending-CI train tests passed on twins. Read the DAG, host adapter and retrospective and ran the independent local Git fixture. **Inferred** ownership and contract-test priorities. **Unverified** live host rules, concurrent conductors, production approval and multi-repository rollout. **Confidence** high for the fixture and static behavior, medium for live exposure.
