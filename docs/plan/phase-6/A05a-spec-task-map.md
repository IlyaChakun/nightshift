# Record which tasks one spec made

State: done

Source: [audit #336](https://github.com/IlyaChakun/nightshift/pull/336). The `spec.expanded` event of the parent task (and `spec.scheduled` for a one-task spec) now carries `{"tasks": {"<spec task id>": <book task id>}}`, written in the transaction that creates the siblings, so tasks without a dependency edge still belong to their spec. A retry finds the marker and writes nothing.

Check with `NIGHTSHIFT_TEST_PG=… go test ./internal/control/dispatch -run TestApprovalQueuesLeadTasksWithTheirSchedule`. The book is the only store involved, so the Postgres test is the real check.
