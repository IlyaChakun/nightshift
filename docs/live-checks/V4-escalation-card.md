# An escalation reaches the owner as a decision card

Plan item V4 of `docs/plan/mvp-validation/`; design: `docs/design/02-control.md`, "Task budget" (a "Decision needed" card on `escalated`).

Failed run, 2026-11-28 on 4e06a14 (task #1008, `V4-A-1008.txt`): the acceptance Verifier escalated because two hidden scenarios needed what a browser cannot show; the move was recorded as `budget_exhausted`, and the last chat card was the Approve reply. The job also started a subagent. Fix: a role's escalation moves as `escalate`, every move to `escalated` queues one "Decision needed" card (reason, pull request, preview and screenshots of an acceptance job, buttons Continue, Revisit the spec, Cancel), acceptance turns refuse `Agent`, and the `hidden-scenarios` skill keeps diff, byte and log criteria out of the scenarios.

Rerun, 2026-11-28 on da6601b (task #1009, `V4-A2-*`, by the Mac thread's account): the card, the `escalate` label, the Cancel button, a browser-only acceptance and no `Agent` call passed; Lead still wrote three scenarios a browser cannot check (it loaded no skill), the card withheld the whole reason, listed the same screenshot twice, and one Slack click left no trace. Fix: the rule is in `roles/lead.md` and `nightshift` sends such scenarios back once, then drops them; the card names the scenario ids; identical files show once; clicks are logged received or ignored.

Run on a real stack after an upgrade to the merge commit. Expected:

- the hidden scenarios Lead writes for the V4 idea are all observable in a browser on the preview;
- if a task escalates (any role), the task's Slack thread gets one "Decision needed" card in Russian with the reason, and `nightshift status --task <id>` shows the move `escalate`, `job_failed`, `budget_exhausted` or `infra_failed`, matching the cause;
- a click on one of the card's buttons moves the task (`building`, `grilling` or `canceled`);
- the acceptance job's transcript has no `Agent` call;
- the book has no `hidden.dropped` event for the V4 task, or only after one `lead.rejected` naming those ids;
- the worker log shows `chat click received` for each button press.

The unit tests in `internal/control/dispatch/escalation_integration_test.go` and `TestAnAcceptanceTurnGetsOnlyTheBrowserTool` cover the logic without a model.

Status: pending
