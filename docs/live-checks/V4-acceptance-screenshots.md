# An acceptance job keeps its screenshots

Plan item V4 of `docs/plan/mvp-validation/`; decision: ADR 0029.

Run on a real stack after an upgrade to the merge commit: rebuild the worker image, then let a synthetic UI task reach Verifier acceptance on its preview. Expected:

- the worker image prints the pinned `playwright-mcp --version` at build;
- the job's transcript shows the turn ran in `dontAsk` mode with `mcp__browser__*` calls that succeeded, and no `Bash`, `Write`, `Edit`, `NotebookEdit` or `browser_evaluate`;
- the evidence card in chat shows the PNG screenshots the job saved, and the book holds them as artifacts of the job;
- the egress-preview proxy logs only preview hosts for the job.

The unit tests `TestAnAcceptanceJobKeepsTheScreenshotsOfItsBrowser` and the argument tests in `internal/execution/harness/claude/browser_test.go` cover the logic without a model.

Failed run, 2026-11-28 on bc5f84e (task #1007, `V4-A-1007.txt`): acceptance job 17 died after 1 s on a chown of the evidence directory, which the worker's capabilities refuse. Fix: the runner opens the directory without a chown; `TestTheEvidenceDirectoryWorksWithTheWorkersCapabilities` runs it with the worker's capability set, in the CI test job as root.

Status: pending
