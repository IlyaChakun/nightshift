# Review evidence when publication resumes

- What. [A01r](../plan/phase-6/A01r-resume-evidence.md) reconciles tasks that resume in `ready` or `merging`. Source: [audit #332](https://github.com/IlyaChakun/nightshift/pull/332).
- Run. On an isolated upgrade rehearsal with a disposable test repository, resume a task whose open PR lacks review evidence. Observe the task and PR after the first conductor tick. Approve the recorded revision as the configured GitHub reviewer and observe the next ticks. Include a second task whose merge reached GitHub before its release was recorded.
- Expect. The open PR waits in `human_review` with a pending status. Approval lets it continue. The PR already merged on GitHub resumes from its merge and completes the release without another merge.
- Status: pending.
