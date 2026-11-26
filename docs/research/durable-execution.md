# Durable execution

Format: `docs/research/README.md`. Decision: ADR 0018. Read 2026-11-26.

## What a long-running factory needs

State that survives a crash; retries with backoff; timers and waits (limits, `not_before`, human buttons); cancellation; side effects that are safe to repeat; fairness across tenants; one place to see what is stuck. Systems differ in who provides each.

## Comparison

| | Temporal | DBOS Transact (Go) | Restate | River (free) | River Pro | LangGraph | Nightshift core |
|---|---|---|---|---|---|---|---|
| Needs | 4 services + Postgres, MySQL or Cassandra | Postgres | one server binary, own log | Postgres | Postgres | a checkpointer store (Postgres) | Postgres |
| Language | Go SDK among others | Go library, pre-v1 | Go SDK 1.0 | Go | Go | Python (JS apart) | Go |
| Licence | unverified | MIT (pkg.go.dev) | server BSL, SDK MIT | MPL-2.0 (summary) | paid | MIT | ours |
| Resume after a crash | replay of history | from the last completed step | journal | retries of a job | workflows | from a checkpoint | book, journal, handoff |
| Timers, signals | yes | scheduling, notifications | yes | snooze, cron | durable periodic | interrupts | polling tick, outside-event watch |
| Fan-out | child workflows | queues | yes | no | workflows | graph | tasks and dependencies |

Sources: [Temporal persistence](https://docs.temporal.io/temporal-service/persistence), [Temporal upgrade](https://docs.temporal.io/self-hosted-guide/upgrade-server), [DBOS](https://docs.dbos.dev/golang/programming-guide), [Restate](https://restate.dev/), [River](https://riverqueue.com/), [LangGraph persistence](https://docs.langchain.com/oss/python/langgraph/persistence). Cells marked "summary" come from a model-extracted page. Temporal's licence and the DBOS and Restate maturity beyond what the pages state are unverified.

## Properties and who provides them

- **Idempotency**: no system provides it for free. Temporal and ADK leave it to the activity or tool; DBOS and Restate record step results but an external call can still repeat. We keep outbox keys and add R2.
- **Step timeouts**: Temporal has per-activity timeouts. We have an agent-job budget in minutes and no timeout on conductor steps (R1).
- **Cancellation**: `nightshift cancel` ends a task; a git operation that keeps no lock after cancel is open (plan M2).
- **Failure recovery tests**: Temporal replays histories; we test with scenarios, and X1 and X2 add a liveness property and fault injection.

## Proposal

R1, R2, R3 in `docs/plan.md`; the revisit triggers are in ADR 0018.
