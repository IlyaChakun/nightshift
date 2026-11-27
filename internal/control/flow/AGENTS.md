# internal/control/flow

The task state machine of design section 7 (`flow.go`, pure) and the `Mover` that applies it
in the book (`move.go`).

- `Next` and `Apply` are pure: no clock, no database. Every transition is an entry in the
  `edges` map; add a state or event there and in the constants, then update the diagram in
  design section 7 (and `docs/design.html`).
- State groups (driven, slot, in progress, waits for the owner, ended) live only in `traits`;
  other packages call `Names`. A new state fails `exhaustive` until it is classified.
- Change a task's state only through `Mover.Apply`: one transaction does the compare-and-set
  on (state, version, head_sha) and appends the `task.moved` event. A lost race returns
  `ErrConflict`; reload and decide again. Never write `book.tasks.state` anywhere else.
- Rules that live in `Next`, not in `edges`: `Cancel` works from any non-terminal state except
  `released` and `rolled_back`; `BudgetExhausted` works only in working states;
  `Merge` needs the solo profile and `HandOff` the team profile; `Accepted` goes to
  `human_review` when the review policy says so; a return event escalates when the budget is
  spent, and `Returned` also at `MaxReturns`.
- Traps:
  - The `Mover` takes `Profile` and `NeedsHumanReview` from the caller: pass the tenant's
    profile and the policy verdict.
  - Only `Returned` increments `Returns`; the other return events do not. `Continue` and
    `Revisit` from `escalated` reset it and the budget flag.
  - `Exhausted` checks iterations and tokens only. The hours limit belongs to the running job.
  - `Terminal` states accept no event, not even `Cancel`.
  - Canceling a task also cancels its jobs in the same transaction (`record`).
  - `Stricter` ranks decisions that arrive at once; give a new decision event a rank there.
- Tests: `flow_property_test.go` (rapid) checks invariants; the integration test needs
  Postgres. A flow change also needs a T4 scenario.
