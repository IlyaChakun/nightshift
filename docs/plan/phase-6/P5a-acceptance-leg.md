# Run the Verifier's acceptance on the preview

State: open

Source: design section 6 (Verifier, acceptance mode) and phase 6 "Verifier acceptance through Playwright". Found on 2026-11-27: the gate moves a task from `acceptance` to `ready` or `human_review` at once (`internal/dispatch/gate.go`, `gate`), and `dispatch.HiddenScenarios` has no caller outside tests, so no acceptance job runs and the hidden scenarios are never used. A live acceptance run (P5c) cannot happen before this.

What to do:

- When a task reaches `acceptance` and the tenant has a Verifier, queue a Verifier job in `acceptance` mode with the preview endpoint and the task's hidden scenarios as inputs (`jobsapi.VisibleInputs` already keeps `hidden_scenarios` for the Verifier only), on the task's head.
- `pass` moves the task on with `accepted`; `return` moves it back to `building` with `scenario_failed` and the behaviour findings only; `escalate` escalates. A task without hidden scenarios or without a preview keeps today's path.
- A stale verdict (another head or preview) is not consumed.

Check: a T4 scenario on the `fake` harness and the deploy twin: one task passes acceptance, one is returned with a finding that names no scenario step, and Builder's next job inputs hold no `hidden_scenarios`.
