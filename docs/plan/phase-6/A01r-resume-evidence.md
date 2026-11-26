# Reconcile review evidence for in-flight tasks after upgrade

State: proposed

Source: [audit #332](https://github.com/IlyaChakun/nightshift/pull/332). For existing `ready` and `merging` tasks, confirm review evidence or an authorized decision before resuming publication. Preserve recovery for revisions already merged.

Check with Postgres and a forge twin across restart. A01b separately checks approval against the host head and the final rebased candidate.
