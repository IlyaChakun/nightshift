---
status: accepted
date: 2026-11-26
---

# Watch every harness run from outside with one supervisor

## Context and Problem Statement

A role runs as a model CLI or an API loop. Inside one run the factory enforced only a wall-clock limit; `Budget.Tokens` was passed and never read, there was no cap in dollars, no stall limit and no check for a tool call that repeats, and killing a CLI left the shells and test runners it started. ADR 0018 found that neither Temporal nor a graph framework sees inside a run, so the control has to sit where the run starts. The owner approved the supervisor on 2026-11-26 and asked that it be modular and tunable per tenant.

## Considered Options

- A supervisor in front of the `Harness` interface that works on normalized events from the adapters.
- Each adapter enforces its own limits with the flags of its CLI (a turn limit, a budget flag).
- Own the agent loop (the `api` harness) and enforce everything inside it, as the Attractor coding-agent-loop spec does.
- Leave it to a workflow engine's timeouts and retries (Temporal).

## Decision Outcome

Chosen option: the supervisor, because one set of rules then serves Claude, Codex and any later CLI, and a harness that cannot stream still gets the process-level rules.

- Adapters emit `output`, `tool_call` and `usage` events through `RunRequest.Observe`. `claude` runs with `stream-json`; `codex` already printed JSON events and we now read them as they come.
- `supervise.Supervisor` wraps a harness with a `Policy` (`StallAfter`, `RepeatLimit`, `Tokens`, `CostUSD`; zero switches a rule off). Rules implement `Rule`, and `Supervisor.Extra` takes more.
- A stopped run returns the exit `budget` with `{"stopped": reason}`. No new exit value, so the jobs API and the book stay as they are.
- `harness.StopGroup` puts the CLI in its own process group; on cancel the group gets SIGTERM and, after five seconds, SIGKILL.
- Limits resolve in layers, each key overriding the one before: the worker's default, the role (`limits` in `roles/*.yaml`), the tenant (`budget.run` in `tenant.yaml`). An absent key inherits; `0` switches a rule off. A client who wants longer runs sets its own `budget.run`, with no code change.

### Consequences

- Good: a hang, a loop or a runaway spend costs minutes, not the whole turn budget; the same rules hold for every harness.
- Bad: the stream formats are the CLIs' own and can change (T8 covers it); the token meter counts what the stream reports per message and may differ from the provider's bill.
- Not verified: the formats against a live `claude` and `codex` of the pinned versions; this needs a run on the Mac.

## Pros and Cons of the Options

- Own loop: full control, but we would write and maintain a coding agent. Kept as a later `api` harness.
- Flags per adapter: the CLIs differ in what they offer and none caps dollars on a subscription; this leaves the rules in three places.
- Temporal: see ADR 0018.
