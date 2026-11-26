# Tear down the preview of an ended task

The [phase 6 P6a item](../plan/phase-6/README.md) removes a task's preview once the task is released, rolled back, merged by the team, canceled or rejected.

On the stable instance with the `deploy` profile on, take one task of a test tenant with a `compose-local` preview to `human_review` and note the compose project `ns-<tenant>-<task>` in `docker compose ls`. Cancel it with `nightshift cancel <id>`. Within the sweep interval (`teardownEvery` in `internal/dispatch/tick.go`) the project is gone from `docker compose ls`, the log shows `preview torn down` for the task, and `book.deploy_steps` has no `up` row for it. Repeat with a task that reaches `released`. With `capacity: { previews: 1 }` in `tenant.yaml`, a second task waits in `preview` (`nightshift status`) while the first is in `human_review`, and its preview comes up after the first is canceled.

Status: pending
