# Revert the change after a rollback

The [phase 6 P6c item](../plan/phase-6/README.md) takes a rolled-back change out of the base branch through a revert pull request and freezes the repository's train until it merges.

On a test tenant with a `compose-local` release and an earlier healthy revision, ship a task whose production smoke fails (for example a healthcheck that never passes). Expect the task in `rolled_back` and production back on the earlier revision. Within a few ticks a pull request `Revert <task title>` from `ns/<task>-revert` opens on the code host. A second task that reaches `merging` meanwhile stays there. After the revert's checks pass the factory merges it, the task moves to `building` with a `release.rolled_back` finding, and the second task's train goes on. Record the task IDs, the pull request links and `nightshift status --task <id>`.

Status: pending
