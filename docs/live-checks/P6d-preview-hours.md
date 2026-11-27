# Tear down a waiting task's preview after preview_hours

The [phase 6 P6d item](../plan/phase-6/README.md) removes the preview of a task that waits after acceptance (`human_review`, `handed_off`, `ready`, `merging`) once `capacity.preview_hours` have passed since its last move out of `acceptance` or into `human_review`.

On the stable instance with the `deploy` profile on, set `capacity: { preview_hours: 1 }` in a test tenant's `tenant.yaml` and take one task with a `compose-local` preview to `human_review`; note `ns-<tenant>-<task>` in `docker compose ls`. Within one sweep (`teardownEvery` in `internal/control/dispatch/tick.go`) after the hour has passed, the project is gone from `docker compose ls`, the log shows `preview torn down`, `book.deploy_steps` has no `up` row for the task, and `nightshift status` still shows it in `human_review`.

Status: pending
