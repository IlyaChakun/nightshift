# Approval follows the rebased patch

- What. [A01b](../plan/phase-6/A01b-approval-follows-patch.md) sends a task back to human review when the train's rebase changes the patch-id. Source: [audit #332](https://github.com/IlyaChakun/nightshift/pull/332).
- Run. On a disposable test repository, approve a task's PR, then push a commit to `main` that changes a context line next to the task's change without a conflict. Let the train take the task.
- Expect. The PR is not merged. The task is in `human_review` with a pending review status on the rebased head. Approving that head merges and releases it.
Status: pending
