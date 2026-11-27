# Keep an approval only for the patch it was given to

State: open

Source: [audit #332](https://github.com/IlyaChakun/nightshift/pull/332). The train journals a rebase as done only when it started in the same run and kept the patch-id. A changed patch or a resumed rebase returns the task to human review at the rebased head; it merges after that head is approved. See [ADR 0021](../../adr/0021-review-and-release-evidence.md).

Check with `go test ./internal/control/train` and `go test ./internal/control/dispatch -run TestAPushedRebaseWithoutJournalGoesToReviewAtTheHostHead`. The [live check](../../live-checks/A01b-approval-follows-patch.md) is pending.
