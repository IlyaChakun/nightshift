# An acceptance job keeps its screenshots

Plan item V4 of `docs/plan/mvp-validation/`; decision: ADR 0029.

Run on a real stack after an upgrade to the merge commit: rebuild the worker image, then let a synthetic UI task reach Verifier acceptance on its preview. Expected:

- the worker image prints the pinned `playwright-mcp --version` at build;
- the job's transcript shows `mcp__browser__*` calls and no `Bash`, `Write`, `Edit` or `NotebookEdit`;
- the evidence card in chat shows the PNG screenshots the job saved, and the book holds them as artifacts of the job;
- the egress-preview proxy logs only preview hosts for the job.

The unit tests `TestAnAcceptanceJobKeepsTheScreenshotsOfItsBrowser` and the argument tests in `internal/execution/harness/claude/browser_test.go` cover the logic without a model.

Status: pending
