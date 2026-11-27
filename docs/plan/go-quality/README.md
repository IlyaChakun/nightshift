# Stream — Go code quality

Owned by the session working on this stream; another session proposes an item as a new file here. Rules and limits: `docs/plan.md`. Source: the owner, 2026-11-27: review and strengthen the Go engineering standards so engineers and agents can change the code safely, with no enterprise patterns. The findings are in `docs/research/go-quality-audit.md`. The owner chose this order on 2026-11-27.

Done when: every item below is merged with its test, each new rule names its check, and `standards/industry/go/style.md` holds only tenant-general rules.

## Status

- Last done: G10: the worker's `stop_grace_period` is 60 s, above `runner.DeliverTimeout` (`deploy/compose_test.go`); live check in `docs/live-checks/G5-shutdown-reason.md`. G9 (#464): a repeated Lead step writes its events once. G7 (#463), G6 (#462), G4 (#460). G5 is merged, live check pending.
- Next: G8, then G11.
- Open: G8: the owner chose on 2026-11-27 to give the agent `/standards` read-only (option 1 of the item).

## Rules for this stream

- One item per PR, under ~400 changed lines, behavior unchanged unless the item fixes a defect; a runtime fix comes with a regression test written first.
- A new rule is a test or an existing linter before it is prose. No new linter or dependency unless the existing ones cannot catch a reproducible problem.
- No repository-wide renames, no new packages for their own sake, no length or count targets.
- Check open PRs of other threads on the same paths before each item.

## Items

- [x] G0. `docs(research): Go code quality audit` — findings F1–F9, which rules reach an agent, the standards delta
- [x] G1. `fix(dispatch): no task left in doc_checks` — an integration test where the second document move fails, then a tick; the conductor drives `doc_checks` like `grilling`, so the unconsumed Lead job finishes the moves (F2)
- [x] G2. `refactor(flow): one owner for task-state classes` — `flow` classifies every state with an exhaustive switch; the 8 lists in dispatch, doors, periodic and summary become calls; same members as today except where F1 found a miss, each named in the PR (F1)
- [x] G3. `chore(lint): enable errchkjson` — fix its 6 issues in production code (7 more in tests are excluded: literal test data) and drop `jobserver.mustJSON`; the existing `//nolint:errchkjson` directives start to mean something (F5)
- [x] G4. `docs(standards): tenant Go style apart from Nightshift's` — `style.md` loses `make verify` and Nightshift contracts; its tool table describes `standards/industry/go/files/.golangci.yml`; a test fails when the table names a linter the template does not enable (both cases tested); the Nightshift rules move to `internal/AGENTS.md` and plane files (F4)
- [ ] G5. `fix(runner): a failed job keeps its reason on shutdown` — a regression test with a cancelled context; the `job.error` event goes on the same detached context as `Complete`; one line in `internal/execution/AGENTS.md`; merged, live check pending (F9)
- [x] G6. `refactor(dispatch): a named result for gate steps` — one small type for "go on or hold" in the gate chain; existing gate and preview integration tests stay green (F6)
- [x] G7. `test(arch): no exported mutable package variables` — an AST test with positive and negative fixtures; unexport or copy the four slices. After A7 PR #449, which edits archtest (F7)
- [ ] G8. `feat(runner): standards reach the agent` — serve `/ns/standards` or give the agent `/standards` read-only, plus a runner test that every "details:" path in the checklist is readable; needs the owner's choice (F3, F8)
- [x] G9. `fix(dispatch): a repeated Lead step writes its events once` — re-running `consumeLead` after a failure appends `lead.reply`, `lead.docs` and `lead.spec` again, and Lead reads the repeat in its next transcript; key the writes by job id, with a regression test (G1 review)
- [x] G10. `fix(deploy): the worker's stop grace covers result delivery` — the worker service sets no `stop_grace_period` (Docker default, unverified: 10 s) while `runner.DeliverTimeout` is 30 s; set a grace above it in `deploy/docker-compose.yml` with a compose test (G5 review)
- [ ] G11. `fix(dispatch): the other repeated consumption events are written once` — `lead.rejected` (counts toward Lead's one correction), `lead.failed`, `patch.rejected` and the research return (counts toward `maxResearchReturns`) still use `AppendEvent`, so a consumption repeated after a failed `ConsumeJob` writes them twice; move them to `AppendJobEvent` with a regression test each (G9 review)
