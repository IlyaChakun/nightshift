# Run the Verifier's acceptance on the preview

State: open

Built on twins; live check pending: [P5a](../../live-checks/P5a-acceptance-leg.md).

Source: design section 6 (Verifier, acceptance mode) and phase 6 "Verifier acceptance through Playwright". Found on 2026-11-27: the gate moves a task from `acceptance` to `ready` or `human_review` at once (`internal/dispatch/gate.go`, `gate`), and `dispatch.HiddenScenarios` has no caller outside tests, so no acceptance job runs and the hidden scenarios are never used. A live acceptance run (P5c) cannot happen before this.

What to do:

- When a task reaches `acceptance` and the tenant has a Verifier, queue a Verifier job in `acceptance` mode with the preview endpoint and the task's hidden scenarios as inputs (`jobsapi.VisibleInputs` already keeps `hidden_scenarios` for the Verifier only), on the task's head.
- `pass` moves the task on with `accepted`; `return` moves it back to `building` with `scenario_failed` and the behavior findings only; `escalate` escalates. A task without hidden scenarios or without a preview keeps today's path.
- A stale verdict (another head or preview) is not consumed.

Check: a T4 scenario on the `fake` harness and the deploy twin: one task passes acceptance, one is returned with a finding that names no scenario step, and Builder's next job inputs hold no `hidden_scenarios`.

Built: `internal/dispatch/acceptance.go`; T4 in `internal/dispatch/acceptance_integration_test.go`. The scenarios reach the Verifier in its job prompt and inputs, not yet through a file in the job tmpfs as design section 6 says (the runner passes no inputs to the agent).

Open after the independent review (2026-11-27): a preview that breaks between `preview` and the acceptance job makes every tick fail with an error instead of sending the task back; an unconsumed review job left when a task reaches `acceptance` blocks the acceptance job; a stale verdict is matched by head only, not by preview URL or scenario set; acceptance findings stay `handed_off` after a later pass.
