# Reconcile review evidence for in-flight tasks after upgrade

State: open

Source: [audit #332](https://github.com/IlyaChakun/nightshift/pull/332). For existing `ready` and `merging` tasks, confirm review evidence or an authorized decision before resuming publication. Preserve recovery for revisions already merged.

Postgres and the forge twin cover the old task states, a new decision and an already merged PR. The [live restart check](../../live-checks/A01r-resume-evidence.md) is pending. [A01b](A01b-approval-follows-patch.md) checks approval against the final rebased candidate, including a rebase push that landed before its journal write.
