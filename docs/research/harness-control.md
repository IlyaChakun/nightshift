# Controlling any harness from outside

Format: `docs/research/README.md`. Decision: ADR 0018. Read 2026-11-26.

## Problem

A role runs as a model CLI or an API loop. A hang, a loop of identical tool calls or a retry storm burns tokens and money. Neither a workflow engine nor a graph library sees inside one run, so the control has to sit where the run is started.

## What we enforce today

| Control | State | Where |
|---|---|---|
| Wall-clock limit per turn | enforced | `Budget.Minutes`, `claude.go`, `codex.go`, `runner.go` |
| Token limit per turn | not enforced | `Budget.Tokens` is passed and never read by an adapter |
| Dollar cap | not enforced | cost is read after the run ends |
| Idle (stall) limit | missing | the adapters read output only at exit (`--output-format json`) |
| Repeated tool call | missing | no tool events reach us |
| Kill of the whole process tree | missing | `exec.CommandContext` kills the CLI, not its children |
| Cancel with a grace period | done for git (R1), not for harnesses | `gitops.go` |
| Conductor step timeout | done (R1) | `tick.go` |

## What each design controls

| Design | Who runs the agent loop | Stuck agent | Cost cap | Source |
|---|---|---|---|---|
| Attractor coding-agent-loop spec | its own loop and LLM client | turn and tool-round limits; loop detection (window 10, patterns of 1 to 3 calls, a steering message); abort kills the process group (SIGTERM, then SIGKILL after 2 s); every command has a timeout | none; "cost tracking and budgets" is listed as outside the spec | spec read in full: lines 1 to 1467 |
| Attractor pipeline spec | a handler per node; the LLM backend is an interface | per-node `timeout`, `max_retries` with backoff, goal gates, a manager loop that observes a child run and may steer it | none | spec read in part: sections 2, 3, 4.5, 4.11, 6, 9.6, 9.7 |
| OpenAI Symphony | Codex app server | `stall_timeout_ms` 300000, `turn_timeout_ms` 3600000, `max_turns` 20 | none, tokens counted | page read 2026-11-26 |
| Temporal | not its job; an activity is a black box | Start-To-Close and Heartbeat timeouts; default retries unlimited | none | docs read 2026-11-26 |
| LangGraph | a node | one limit: 10,007 super-steps by default | none | docs read 2026-11-26 |
| StrongDM site | not stated | "apply more tokens"; no stuck-agent or timeout text | a spend target, no cap | four pages read 2026-11-26 |

The Attractor README says to own the agent loop and the LLM client "so you have a strong foundation". That is the only design here that owns the loop, and it is why its loop detector can exist. We wrap CLIs, so we get only what a CLI prints.

## What a supervisor can do for any harness

Three levels, each needing less from the adapter than the next.

1. **Process level, nothing from the adapter.** Wall-clock limit, kill of the process group (stop signal, grace period, kill), no output for N minutes means stall. This works for Claude, Codex, a DeepSeek or Llama CLI, anything.
2. **Event level, the adapter emits normalized events.** Claude has a streaming JSON output and Codex has JSON events (formats unverified against our versions). The adapter turns them into output, tool call and usage events. The supervisor then enforces a token meter, a dollar cap (tokens times the price table) and a repeated-call detector. A harness that cannot stream is limited to level 1.
3. **Loop level, we own the loop.** The `api` harness (not built) would run our own loop, as Attractor does, and gets every control for free, at the price of building and maintaining a loop.

A control that cannot be enforced by a given harness is reported, not faked: the run record says which limits were enforceable.

## Do we need a framework?

| Option | Would replace | Would not replace | Verdict |
|---|---|---|---|
| Temporal | the conductor tick, retries, park and wake, the merge train step journal | stall, loop and token control inside a run; our book stays the source of truth for tasks; adds a cluster or paid Cloud (owner decision) | not now; spike proposed (decision 15) |
| LangGraph or LangChain | a flow graph plus checkpointer | CLI harnesses (it expects model calls it makes itself); a second language and runtime; one step limit | no |
| eino | a Go graph with interrupt and resume | the store (we write a key-value store ourselves); no step or token limit; Go only | no |
| ADK | agents and resume | resume is documented for Python and Kotlin, Go unconfirmed; tools run at least once | no |
| Own supervisor (S1) | nothing existing | it adds the missing controls | yes, as S1 |

The reason is the same for all: each helps with state and waiting, which the book and River already provide, and none controls the inside of a run.

## The timeline view the owner asked for

Temporal's event history and web UI are real and are the strongest argument for it. We have the same data in `events` and job results. S2 builds a per-task timeline from them, which needs no new service. The spike (decision 15) measures whether that is enough.

## Not read

Attractor pipeline spec sections 5 (partly), 7, 8, 10 to 12; the unified LLM spec outside retry and timeout; StrongDM product and technique sub-pages and `/weather-report`; LangGraph fault-tolerance page; ADK retries, timeouts and cancel pages; Temporal Worker Versioning. Claude and Codex streaming formats were not checked against our pinned versions.
