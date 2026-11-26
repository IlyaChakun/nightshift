# Durable execution

Format: `docs/research/README.md`. Decision: ADR 0018. Read 2026-11-26.

## What a long-running factory needs

State that survives a crash; retries with backoff; timers and waits (limits, `not_before`, human buttons); cancellation; side effects that are safe to repeat; fairness across tenants; one place to see what is stuck. Systems differ in who provides each.

## Comparison

| | Temporal | DBOS Transact (Go) | Restate | River (free) | River Pro | LangGraph | Nightshift core |
|---|---|---|---|---|---|---|---|
| Needs | 4 services + Postgres, MySQL or Cassandra | Postgres | one server binary, own log | Postgres | Postgres | a checkpointer store (Postgres) | Postgres |
| Language | Go SDK among others | Go library, pre-v1 | Go SDK 1.0 | Go | Go | Python (JS apart) | Go |
| Licence | server MIT (README, pkg.go.dev); Go SDK not stated | MIT (pkg.go.dev) | server BSL, SDK MIT | MPL-2.0 (summary) | paid | MIT | ours |
| Resume after a crash | replay of history | from the last completed step | journal | retries of a job | workflows | from a checkpoint | book, journal, handoff |
| Timers, signals | yes | scheduling, notifications | yes | snooze, cron | durable periodic | interrupts | polling tick, outside-event watch |
| Fan-out | child workflows | queues | yes | no | workflows | graph | tasks and dependencies |

Sources: [Temporal persistence](https://docs.temporal.io/temporal-service/persistence), [Temporal upgrade](https://docs.temporal.io/self-hosted-guide/upgrade-server), [DBOS](https://docs.dbos.dev/golang/programming-guide), [Restate](https://restate.dev/), [River](https://riverqueue.com/), [LangGraph persistence](https://docs.langchain.com/oss/python/langgraph/persistence). Cells marked "summary" come from a model-extracted page. The Go SDK licence and the DBOS and Restate maturity beyond what the pages state are unverified.

## Properties and who provides them

- **Idempotency**: no system provides it for free. Temporal and ADK leave it to the activity or tool; DBOS and Restate record step results but an external call can still repeat. We keep outbox keys and add R2.
- **Step timeouts**: Temporal has per-activity timeouts. We have an agent-job budget in minutes and no timeout on conductor steps (R1).
- **Cancellation**: `nightshift cancel` ends a task; a git operation that keeps no lock after cancel is open (plan M2).
- **Failure recovery tests**: Temporal replays histories; we test with scenarios, and X1 and X2 add a liveness property and fault injection.

## Proposal

R1, R2, R3 in `docs/plan.md`; the revisit triggers are in ADR 0018.

## Temporal for agent work (second look, 2026-11-26)

- **Shape.** An LLM call or a coding-agent turn is an Activity; the workflow code around it must be deterministic and replayable ([OpenAI Agents SDK integration, 2025-07-30, GA 2026-03-23](https://temporal.io/blog/announcing-openai-agents-sdk-integration); [sandboxes, 2026-04-16](https://temporal.io/blog/introducing-temporal-and-agentic-sandboxes-openai-agents-sdk)). The official integration is Python and the OpenAI Agents SDK. No Temporal page names Claude Code, Codex or another coding-agent CLI; a third-party guide says there is no official integration and that a turn run as one activity can be billed again after a retry and that retries have no attempt limit by default ([Omnara guide, 2026-10-28, a competitor: unverified](https://www.omnara.com/guides/temporal-ai-agents)).
- **What it gives.** A durable event history per run, replay for debugging, a Web UI, waiting without compute ([event history](https://docs.temporal.io/encyclopedia/event-history/event-history-go.md)); a dev server with the UI on port 8233 (`temporal server start-dev`, [README](https://raw.githubusercontent.com/temporalio/temporal/main/README.md)); the server is MIT, v1.30.0 published 2026-02-04 ([pkg.go.dev](https://pkg.go.dev/go.temporal.io/server@v1.30.0)).
- **What it costs.** Determinism rules for workflow code and versioning with `workflow.GetVersion` or Worker Versioning for runs in flight; a non-deterministic change makes a workflow task fail and retry over and over ([workflow definition](https://docs.temporal.io/workflow-definition), [Go versioning](https://docs.temporal.io/develop/go/workflows/versioning)). History limits (51,200 events or 50 MB) come from the third-party guide only.
- **What it does not give.** A token meter, a loop detector or a spend cap for what happens inside one activity: its levers are timeouts and retries. It sees the CLI as a black box until the activity ends.

## How other factories orchestrate (read 2026-11-26)

| System | Orchestration | Stuck agents | Cost cap | Harness |
|---|---|---|---|---|
| [Open SWE](https://github.com/langchain-ai/open-swe) | LangGraph durable threads, a sandbox per thread | only flaky-CI reruns | not stated | model and sandbox are options |
| [OpenAI Symphony](https://openai.com/index/open-source-codex-orchestration-symphony/) | in-process scheduler, tracker as state machine, recovery without a database; "general-purpose workflow engine" is a non-goal | `stall_timeout_ms` 300000, `turn_timeout_ms` 3600000, `max_turns` 20, restart with backoff | none, tokens are counted | Codex only |
| [Multica](https://github.com/multica-ai/multica) | Go and Postgres, a daemon that runs agent CLIs | not stated | per-run cost shown, no cap | many CLIs |
| [Majento](https://df.majento.ai/talk/) | dispatcher and a step log in SQLite | a limit on agent-to-agent ping-pong | not stated | per-role models |
| StrongDM (via [Willison](https://simonwillison.net/2026/feb/7/software-factory/)) | not described | not stated | a spend target, no cap | not stated |

None of the five names Temporal on the pages read, and none enforces a cost cap. The one explicit stuck-agent control, Symphony's stall timeout, works on the output stream of the agent, whatever the agent is.
