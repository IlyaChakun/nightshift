# A person's Merge on GitHub releases the verified head

The [MVP validation V4 item](../plan/mvp-validation/README.md) found that a Merge pressed on GitHub, instead of an Approve review, ended a verified solo task in `merged_by_team` without a release. Now an approver's Merge of the head the factory verified counts as the approval (`humanMerge` in `internal/control/dispatch/gate.go`).

On the stable instance with the `deploy` profile on, run V4 twice on the sandbox tenant, each time up to `human_review` with an evidence card:

1. Approve path: submit an Approve review on GitHub as the owner. Expect `ready`, then `released` in `nightshift status --task <id>`, production on the merge commit, the preview torn down.
2. Merge path: press Merge on GitHub as the owner, with no review. Expect the same end: `approve` and `merge` moves with actor `github:<owner login>`, then `release_done`, and no "ended outside the factory" card.

Optional: merge a third task during `checks`. Expect `merged_by_team`, no release, and a card that says it had not passed the factory's checks.

Status: pending
