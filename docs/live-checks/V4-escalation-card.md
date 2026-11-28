# An escalation reaches the owner as a decision card

Plan item V4 of `docs/plan/mvp-validation/`; design: `docs/design/02-control.md`, "Task budget" (a "Decision needed" card on `escalated`).

Failed run, 2026-11-28 on 4e06a14 (task #1008, `V4-A-1008.txt`): the acceptance Verifier escalated because two hidden scenarios needed what a browser cannot show; the move was recorded as `budget_exhausted`, and the last chat card was the Approve reply. The job also started a subagent. Fix: a role's escalation moves as `escalate`, every move to `escalated` queues one "Decision needed" card (reason, pull request, preview and screenshots of an acceptance job, buttons Continue, Revisit the spec, Cancel), acceptance turns refuse `Agent`, and the `hidden-scenarios` skill keeps diff, byte and log criteria out of the scenarios.

Run on a real stack after an upgrade to the merge commit. Expected:

- the hidden scenarios Lead writes for the V4 idea are all observable in a browser on the preview;
- if a task escalates (any role), the task's Slack thread gets one "Decision needed" card in Russian with the reason, and `nightshift status --task <id>` shows the move `escalate`, `job_failed`, `budget_exhausted` or `infra_failed`, matching the cause;
- a click on one of the card's buttons moves the task (`building`, `grilling` or `canceled`);
- the acceptance job's transcript has no `Agent` call.

The unit tests in `internal/control/dispatch/escalation_integration_test.go` and `TestAnAcceptanceTurnGetsOnlyTheBrowserTool` cover the logic without a model.

Status: pending
