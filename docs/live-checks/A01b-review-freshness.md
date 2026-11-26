# Check review on the current pull request head

Status: pending

Source: [A01b review freshness](../plan/phase-6/A01b-review-freshness.md) and [audit #332](https://github.com/IlyaChakun/nightshift/pull/332).

On an isolated staging tenant, update a pull request awaiting review. Confirm that the task continues waiting for reconciliation. In a separate task, add an authorized request for changes to the revision pending publication. Expect the task to wait for review and record the revision and verdict as evidence.
