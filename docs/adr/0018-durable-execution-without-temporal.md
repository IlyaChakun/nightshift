---
status: accepted
date: 2026-11-26
---

# Keep the Go conductor on the book and River; do not adopt Temporal

## Context and Problem Statement

The owner asked whether the factory should move its orchestration to Temporal, and to add a migration phase if the answer is a clear yes. The factory is a long-running, retry-heavy system with human waits, model limits and a merge train, which is the kind of system durable execution is sold for. Design section 2 (principle 1) and the "Deliberately not doing" list already exclude Temporal; this ADR tests that exclusion against evidence instead of repeating it. The question: does Temporal remove enough hand-built machinery, and avoid enough defects, to justify its cost on a single-laptop `docker compose` instance?

## Considered Options

- Keep the Go conductor on the book (Postgres, CAS transitions, `events`) and River, and harden it with tests and small helpers.
- Adopt Temporal (self-hosted or Cloud): conductor, merge train and outbox become workflows and activities.
- Adopt a lightweight durable-execution library on the same Postgres (DBOS Transact for Go).
- Adopt a durable-execution server with its own storage (Restate).
- Adopt a Python or Go agent-graph framework for orchestration (LangGraph, eino, ADK).

## Decision Outcome

Chosen option: "Keep the Go conductor on the book and River", because every property that Temporal would give us is either already built and tested here or is not what has gone wrong, and Temporal's cost lands exactly on the single-laptop, one-database design. No migration phase is added. Verdict: **no**; revisit on the triggers below.

### What the core already has, against what Temporal gives

| Durable-execution property | Temporal | Nightshift today |
|---|---|---|
| State survives a crash | Event history, replay | The book is the state: `tasks` and `events` in Postgres, CAS transitions (`internal/flow`), a session handoff per (task, role) (ADR 0005) |
| Multi-step side effects resume at the right step | Activities in a workflow | The merge-train step journal reconciles with GitHub after a restart (design section 7, `internal/train/journal.go`) |
| Retries with backoff, unique jobs, timers | Activity retry policy, timers | River: retries, unique and periodic jobs, transactional enqueue with the state change (design section 3) |
| Waiting for a limit, then falling back | Timers and signals | Limit parking and wait-then-fallback (ADR 0012) |
| Idempotent side effects | Left to the activity author | Outbox with idempotency keys (design section 3, `internal/control/outbox/outbox.go`) |
| Outside events (merge or close on GitHub, cancel) | Signals | Outside-merge watch, `nightshift cancel` (plan, phase 3 item 8) |
| Fair claims across tenants | Task queues, no fairness by default | Claims by `budget.weight` with a provider cap at the claim (plan M2) |
| Operator view | Web UI, visibility | Grafana views over the book, one trace per task (design section 3) |

Not covered by us, and given by Temporal: replay-based testing of workflow code, durable timers measured in days without a polling tick, and a ready UI. The first is replaced by the scenario runner and the open items X1 and X2 (a liveness property and seeded fault injection); the other two have not been a problem so far.

### Cost on a single-laptop instance

- **Extra services.** A self-hosted cluster has four independently scalable services (Frontend, History, Matching, Worker) and requires a persistence database; Cassandra, PostgreSQL and MySQL are supported ([persistence docs](https://docs.temporal.io/temporal-service/persistence), read 2026-11-26). The upgrade guide says to run at least two instances of each service and to upgrade one minor version at a time with a schema step ([upgrade guide](https://docs.temporal.io/self-hosted-guide/upgrade-server), 2026-11-26). The one-process development server and SQLite are, in the docs' words, for development and testing only. A production-shaped Temporal on a laptop is therefore four more containers and a second database, or a dev-only setup that we would not trust with the merge train. The compose example of the Temporal docs also starts Elasticsearch ([deployment guide](https://docs.temporal.io/self-hosted-guide/deployment), 2026-11-26).
- **Two sources of truth.** Principle 1 and the book make Postgres the record and the chat and tracker projections of it. A workflow history would be a second record of the same task state, and every transition would have to be written to both, or the book would become a projection of Temporal. That reverses design section 2 and breaks the Grafana views, the digest and `nightshift status`, which read the book.
- **Determinism and versioning.** Workflow code must replay deterministically, and changing it while tasks are in flight needs versioning. The conductor is today a state machine read from the book on a tick; a change to it is a deploy, not a patch of live histories. The factory edits its own code (the stable instance builds the factory), so in-flight tasks across self-upgrades is the normal case, not an edge.
- **Cloud.** Temporal Cloud is usage-priced: the docs list "$50 per million Actions", Business as "the greater of $500/mo or 10% of usage spend", and a Developer plan with no base fee and a 10% charge ([pricing docs](https://docs.temporal.io/cloud/pricing), 2026-11-26). The marketing page quotes "$150 in credits" and "down to $25 per million actions"; the two pages disagree about the Developer tier and we did not resolve it. Cloud needs a paid account and a new egress host: both are "Ask before you do it" items (`AGENTS.md`), and task metadata would leave the laptop. The money is small for our volume; the account, the host and the data location are the cost.
- **Migration effort (inferred, not measured).** The conductor, build, gate, lead, research, deps, handoff and morning flows in `internal/dispatch`, the merge train, the outbox and `internal/flow` would be rewritten as workflows and activities, with the T1 to T5 tests ported. That is of the order of phases 2 and 3 again (the history shows about 190 PRs for phases 0 to 4), plus a freeze of new factory features while it lands.
- **Licence.** The Temporal server is MIT ([README](https://raw.githubusercontent.com/temporalio/temporal/main/README.md), [pkg.go.dev](https://pkg.go.dev/go.temporal.io/server@v1.30.0), read 2026-11-26); the Go SDK licence is not stated on the pages read: **unverified**. Licence is not an objection.

### Strongest counter-argument

The factory keeps growing hand-built durable-execution pieces: the step journal, restart reconciliation, limit parking, the outside-merge watch, claim sweeps. Each defect the owner found by asking (stuck tasks, a merge outside the factory, a return job that starts without the last patch) belongs to the class that durable execution is designed to remove, and each fix costs a PR. This is a real point: the conductor will keep growing.

Why it does not carry the decision today:

- The defects were missing outside events, missing timeouts and missing context, not lost state. Temporal would not have received a "PR merged outside" signal by itself, and it leaves idempotency to the activity author, as ADK's resume page also says of tools (they "run at least once", [ADK resume](https://adk.dev/runtime/resume/), 2026-11-26).
- The class is addressed more cheaply by a liveness property test and seeded fault injection in the twins (plan X1, X2), which would also test a Temporal-based conductor.
- `make drill` passed 22 of 22 on the owner's Mac on 2026-11-25, including restore and restart.

### Alternatives

- **DBOS Transact for Go** is the closest fit if durable execution is ever wanted: a library, Postgres only, workflows and steps recovered "from its last completed step", MIT per pkg.go.dev ([programming guide](https://docs.dbos.dev/golang/programming-guide), [announcement post, 2025-12-19](https://www.dbos.dev/blog/how-we-built-golang-native-durable-execution), [pkg.go.dev](https://pkg.go.dev/github.com/dbos-inc/dbos-transact-golang@v0.5.6-beta), read 2026-11-26). Against it: the version we could read is `v0.5.6-beta` (Sep 2025), the API names differ between the docs and the README (churn), and it would still run next to the book and River, so it adds a third notion of state. Not now; the first candidate if the triggers fire.
- **Restate**: a single binary with its own log, no external database; the server is under the BSL and the SDKs under MIT, the Go SDK 1.0 needs Go 1.24 ([restate.dev](https://restate.dev/), [sdk-go](https://github.com/restatedev/sdk-go), 2026-11-26). It adds a service and a second store, against "one database", and a BSL server against our licence stance. No.
- **River Pro workflows** (fan-out and fan-in on the existing queue) are a paid subscription ([riverqueue.com](https://riverqueue.com/), 2026-11-26); money is an owner decision, and we have no fan-out need yet. No. Resumable jobs and recorded output are on the free tier and were not read (open).
- **LangGraph** is Python; durability needs a Postgres or SQLite checkpointer, and in-memory checkpoints are lost on restart ([persistence docs](https://docs.langchain.com/oss/python/langgraph/persistence), 2026-11-26). A Go service would call it as a sidecar. It also conflicts with ADR 0002 (harnesses, not a framework). No. Its idea, a state snapshot per thread, is our handoff.
- **eino** (Go, Apache-2.0) has graphs and interrupt and resume, but its checkpoint store and crash behaviour were not verified ([eino](https://github.com/cloudwego/eino), 2026-11-26). **ADK for Go** has resume by invocation id, with Go-specific durability unverified ([adk-go](https://github.com/google/adk-go), 2026-11-26). Both would sit next to the `Harness` interface and duplicate it. No.

### What we take instead

- Plan X1 and X2 (liveness property with outside events, seeded fault injection) carry the weight of a replay test.
- Plan R1: a timeout per conductor step (Temporal and ADK both have per-activity timeouts; ours is the open "Later" of M2).
- Plan R2: a test that lists every side effect of the conductor and the train and fails when one has no idempotency key or "already done" check, because the at-least-once rule holds in every system read.
- Plan R3: stop an agent run that repeats the same tool call, a technique of Attractor's coding-agent loop spec.

### Revisit when

Any one of these holds, measured, and then DBOS Transact for Go is the first option to test on a throwaway branch:

- X1 and X2 keep finding lost-state or stuck-task defects after the fixes (more than three confirmed in a month);
- a second orchestrator path appears (a deploy flow, P2, that cannot share the conductor's journal);
- a flow needs durable timers or fan-out across many tasks that a tick cannot serve;
- the owner moves the deployment off one machine, so that a cluster is no longer a cost.

### Consequences

- Good: no new service, dependency, account, egress host or second store; the book stays the one record.
- Good: the effort goes into tests that would also guard a later migration.
- Bad: we keep maintaining our own journal and reconciliation code; the conductor's growth is a standing risk, watched by the revisit triggers.
- Bad: no replay of production histories; the book's `events` give an audit trail, not re-execution.

## Second look (2026-11-26, after the owner's question)

The owner asked for a closer look: Temporal gives a controlled workflow with an audit trail and a UI, an agent can read where a run failed, and the factory must not depend on one harness. The details are in `docs/research/durable-execution.md`. What changed and what did not:

- **Confirmed and added.** The server is MIT. A run's event history, replay and a Web UI are real benefits for audit and debugging. Our book keeps every transition in `events`, with one trace per task, but has no task-timeline view; that part of the benefit can be had with a view over `events` for far less than a migration (plan S2).
- **Harness independence is not an argument for or against.** A Temporal activity would call our `Harness` interface, as the conductor does today. The earlier text named `claude` because it is the harness that ran live; the controls below belong in the runner, for every harness.
- **The cost concern is not met by Temporal.** What happens inside one agent run (a hang, a tool called again and again, tokens spent) is invisible to it until the activity ends; its levers are timeouts and retries, and a retry of a long run can spend the tokens again (third-party claim, unverified). Today the runner enforces only wall-clock minutes: `Budget.Tokens` is read by no harness, and `claude` runs with `--output-format json`. Symphony, the one peer with an explicit stall control, enforces it on the agent's output stream. Plan S1 builds that for every harness.
- **Peers.** Of Open SWE, Symphony, Multica, Majento and StrongDM, none names Temporal on the pages read, and none enforces a cost cap. Symphony states that a general-purpose workflow engine is a non-goal.
- **Verdict unchanged, now with a measurement.** The spike ran on 2026-11-26 (decision 15): a Lead, Builder, Verifier and `OpenPR` workflow on the development server with the `fake` harness. Recovery after a killed worker, retries and a heartbeat stall timeout all work; the history does not keep the errors of failed attempts; a retry repeats a whole agent turn. Details, limits and what was not run: `docs/research/temporal-spike.md`. Result: Temporal is not taken; S1 and S2 carry the useful parts (stall control from the harness event stream, a timeline with every attempt). The spike branch `spike/temporal-experiment` is kept until the owner agrees to delete it.

## Evidence

- Checked: design sections 2, 3, 7, 12; ADRs 0001, 0002, 0012; `docs/plan.md`; `docs/status.md`; the Temporal pages named above, DBOS, Restate, River, LangGraph, eino, ADK pages (read 2026-11-26 through a page-reading helper that returns extracted text, so quotes are not checked against raw HTML).
- Not read: the Temporal Go SDK licence;  the Temporal visibility and versioning pages; eino checkpoint docs; the newest DBOS Go version; River's free resumable jobs.
- Inferred: the migration estimate and the claim that Temporal would not have prevented the defects the owner found.
- Confidence: high that the decision is right for now (cost and design fit); medium on the revisit triggers.
