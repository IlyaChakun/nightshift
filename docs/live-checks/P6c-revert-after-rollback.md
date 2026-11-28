# Revert the change after a rollback

The [phase 6 P6c item](../plan/phase-6/README.md) takes a rolled-back change out of the base branch through a revert pull request and freezes the repository's train until it merges.

On a test tenant with a `compose-local` release and an earlier healthy revision, ship a task whose production smoke fails (for example a healthcheck that never passes). Expect the task in `rolled_back` and production back on the earlier revision. Within a few ticks a pull request `Revert <task title>` from `ns/<task>-revert` opens on the code host, and the task's thread gets one card in the chat language with what failed, the revision live again and the revert pull request link. A second task that reaches `merging` meanwhile stays there. After the revert's checks pass the factory merges it, the task moves to `building` with a `release.rolled_back` finding, and the second task's train goes on. Then repeat with a commit on the base branch that edits the same lines before the revert: expect one blocker card in the task's thread, no revert pull request and the train still held, and after a revert by hand the task in `building`. Record the task IDs, the pull request links and `nightshift status --task <id>`.

Failed run, 2026-11-28 (task #1012, `V5-C-1012.txt` on the Mac, by the Mac thread's account, not read by the thread that recorded it): the rollback to the earlier revision and revert PR #49 worked, but the owner got no chat message about either. Fix: `tellRolledBack` in `internal/control/dispatch/revert.go`, test `TestARollbackTellsTheOwnerOnce`.

Status: failed 2026-11-28 on task #1012 (no rollback card); rerun after an upgrade to the fix
