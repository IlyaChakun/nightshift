# Go code quality audit

Item G0 of `docs/plan/go-quality/`, on `main` at `b1b236f` (2026-11-27). It checks the Go standards and representative code of every plane against Effective Go, Go Code Review Comments and the Google and Uber guides (precedence in `standards/industry/go/style.md`). The backlog and its checks are in the plan folder.

Summary: the base is sound. Lint covers 37 enabled linters plus the 5 standard ones, archtest enforces the plane graph, and `iface` and `interfacebloat` find no bloated or duplicate interfaces. Three gaps matter:

- one business rule (what each task state means) is copied in 8 lists;
- the factory's agents cannot read the Go standard they are pointed to;
- `style.md` mixes tenant rules and Nightshift rules.

## How it was checked

- Read: root and plane `AGENTS.md`, `standards/industry/go/`, both `.golangci.yml` files, `stacks/go/pack.yaml`, ADR 0026 and 0027, `internal/testing/archtest/policy_test.go`, `roles/*.md`, and the code cited below.
- Ran golangci-lint 2.15.0 one linter at a time: `errchkjson` 8 issues, `iface` 0, `interfacebloat` 0, `forcetypeassert` 5 (tests only), `goconst` 50 (mostly test strings, noise). An AST scan listed every package-level `var` outside tests.
- Verdicts: confirmed (seen in code and callers) or plausible (the path exists, no reproducing test yet).

## Findings

**F1. Task-state classes are copied in 8 places and disagree (confirmed).**

- The lists:
  - `flow.activeStates`, which drives the ticker;
  - `dispatch.slotStates`, `stalePreviewStates` and `waitingStates`;
  - `doors.openStates`;
  - `periodic.allStates` ("every state a task can be in");
  - `summary.waitingStates` and `summary.activeStates`.
- `doc_checks` is missing from the first, the door list and `allStates`, but `summary` counts it as active.
- Two lists named `waitingStates` have different members.
- No test ties any list to the 22 states in `flow.go`.

**F2. A task can be left in `doc_checks`, where nothing drives it (plausible).**

- `passDocSteps` (`dispatch/lead.go`) and `docLoop` (`dispatch/docloop.go`) make two `Mover.Apply` calls, each in its own transaction.
- If the second one fails, the task stays in `doc_checks`. It can fail on a step timeout, a shutdown or a database error.
- The comment "a failure in between repeats the steps on the next tick" holds only for ticked states, and `doc_checks` is not ticked.
- The result: no spec card, a Lead job that is never consumed, "working" in the digest and absent from `/status`.

**F3. Builder and Verifier cannot read the Go standard they are pointed to (confirmed in code, live check pending).**

- `standards.Checklist` writes "details: standards/industry/go/style.md".
- The agent runs in the tenant's checkout and gets only the skills directory as an extra directory (`runner.go`, `harness/claude`).
- `/standards` is in the worker image, but the agent is never given access to it.
- So the prose rules never reach factory agents; only lint failures do.

**F4. `style.md` mixes tenant and Nightshift rules (confirmed).**

- The page says "`.golangci.yml`, run by `make verify`". Its tool table names `revive`, `sloglint`, `exhaustive` and other linters.
- The tenant template `standards/industry/go/files/.golangci.yml` enables 9 linters, and `contextcheck` among them is not in ours.
- The interface exception names Nightshift's own contracts.

**F5. `jobserver.mustJSON` has an unreachable fallback (confirmed, low).**

- Its one caller passes a `map[string]string`, which cannot fail to encode.
- 12 call sites carry `//nolint:errchkjson`, but that linter is off, so the directives guard nothing.

**F6. Gate step booleans flip meaning (confirmed readability, no bug found).**

- In `dispatch/gate.go`, one call chain uses five different "true"s: `done` (also "still running"), `goOn`, `back`, `stop` and `wait`.
- One inverted value silently advances or stalls a task.

**F7. Exported mutable package slices (confirmed, low).**

- `memory.FactFiles`, `check.DefaultDocKinds`, `deps.DefaultLicenses` and `standards.Levels`.
- None is mutated today, but any importer could change policy for the whole process.
- `flow.ActiveStates()` shows the fix already used in this repo: return a fresh copy.

**F8. Jobs API routes promised but not served (confirmed).**

- `jobsapi.RouteSearch`, `RouteStandards` and `RouteTask` have no handler, yet `docs/design.md` lists them.
- `/ns/standards` is the designed answer to F3.

**F9. A failed job's reason can be lost on shutdown (plausible).**

- `runner.execute` sends the `job.error` event on the job context and ignores the error.
- `Complete` uses `context.WithoutCancel` "so the result reaches the book".
- When shutdown caused the error, the event fails and the book gets an error with no reason.

**Checked, no defect:**

- `Conductor` size is ADR 0027's decision. Tenant settings arrive both as copied fields and as the `t` argument; tenants are not reloaded, so the copies cannot drift today.
- Ports are consumer-owned: `train.Forge` narrows `codehost.PullRequests`.
- The runner's lease, heartbeat and checkout cleanup own their goroutine and use detached contexts. Keep them as the example.
- dispatch runs 3 inline read queries outside sqlc. Not a defect.

## Which rules reach an agent

| Where | Reaches the agent? |
|---|---|
| `.golangci.yml`, archtest, tests | yes, as failures |
| `internal/**/AGENTS.md` | yes: Claude Code loads it through the `CLAUDE.md` import when it touches the folder (seen in the audit session); Codex reads nested `AGENTS.md` |
| `standards/industry/go/style.md` | only when an agent opens it; factory agents cannot (F3) |
| independent review (`/code-review`) | generic; it does not load `style.md` |

So a Nightshift rule goes into a test or a linter first, then into the plane `AGENTS.md`. Root `AGENTS.md` (1783 of 1800 words) and new prose files are not the place.

## Standards delta

Enforcement levels: 1 compiler, 2 linter, 3 test, 4 review checklist, 5 prose.

| Principle | Rule and when it applies | Level | Change |
|---|---|---|---|
| KISS, YAGNI | no abstraction before a second real caller | 5 | covered |
| DRY of knowledge | a business rule has one owner package; others call it, never re-list it | 1+2: a `flow` method with an exhaustive switch | new (G2) |
| Cohesion, coupling | plane imports and ports | 3 archtest | covered |
| Single responsibility | a "go on or hold" step result is a named type, not a bare bool | 1 | new (G6) |
| Small consumer interfaces | interface in the consumer | 2+4 | covered |
| Minimal public API | no exported mutable package `var`; error values allowed | 3 AST test | new (G7) |
| Terminology | one name means one thing within a plane | 4 | new checklist line |
| Explicit state | enums are named types, switched exhaustively | 2 `exhaustive` | covered |
| Fail-safe | no fallback that hides an error; check encoding errors | 2 `errchkjson` | new (G3) |
| Least privilege | worker isolation, trusted Git | 3 | covered |
| Transactions | moves that must land together share one transaction | 3 | new test (G1) |
| Goroutines, delivery | every goroutine has a stopper; results and their reasons go on a detached, bounded context | 3+5 | new line in `execution/AGENTS.md` (G5) |
| Idempotency, retries | outbox and journals with fixed keys | 3 | covered |
| Tests, compatibility | behavior through the public API; expand-only migrations | 3+5 | covered |

Tenant-general Go stays in `style.md`, and its tool table describes the tenant template. Nightshift-specific rules go into plane `AGENTS.md` files and into tests.

## Not verified

- F2 and F9 have no reproducing test yet; G1 and G5 add them.
- F3 was read from the code, not checked in a running worker.
- The four style guides were not re-read for this audit.
