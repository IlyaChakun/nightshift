---
status: proposed
date: 2026-11-25
---

# Work through model limits: wait first, fall back by a per-role chain

## Context and Problem Statement

The factory should run around the clock while tasks are queued, plan tasks ahead so they run at night, and survive running out of subscription or API quota. Design section 12 says "on a limit error the role switches to the next provider; if the list is exhausted, the task is parked until the reset". The code does not do this yet: a `rate_limit` job ends as `failed`, the conductor escalates the task to the owner, `Router.NoteLimit` has no caller, and limits live only in memory. One empty weekly limit therefore stops a night of work. Which strategy keeps the factory busy without trading away quality or money?

## Considered Options

- Wait and resume: park the task until the provider's reset, then continue
- Fallback only: move to the next provider at once on any limit
- Mix: wait for short resets, fall back along a per-role chain for long ones, with guards on quality and cost
- Buy headroom: raise limits with pay-as-you-go credits and never switch models

## Decision Outcome

Chosen option: "Mix, wait first", because the limits of one provider differ a lot in length and the roles differ in how much a weaker model hurts.

- **A limit is a fact in the book, not a failure.** The runner reports the limit and its reset time; `nightshift` stores it per provider (it survives a restart), puts the job back in the queue without counting an attempt, and moves no task state. Reset time comes from the harness when it knows it (Claude Code's `rate_limit_event` carries `resetsAt`), else from `Retry-After`, else an exponential backoff from 15 minutes to 6 hours.
- **Wait when the reset is near.** If the earliest reset in the role's chain is within `max_wait` (default 2 hours, tenant setting), the task waits. This covers the five-hour kind of limit and rate limits.
- **Fall back when it is far.** Past `max_wait` (typically a weekly limit) the router uses the next provider of the role's chain, but only if its tier allows the task: a fallback provider is marked `tier: fallback` in `providers.yaml`, and a task of blast radius `high` or `critical` never runs on one; it waits for the reset. Lead's talk with the owner never falls back (a cheaper model decides the spec), Verifier may, because a second family is a benefit.
- **Money has a cap.** A fallback provider spends only its pool (`monthly_usd`, `caps`) and a per-task ceiling; an empty pool is a limit like any other.
- **Gates do not change.** A weaker Builder cannot merge a bad change: L1–L3, the Verifier and the owner's Approve stay as they are. The price of a weaker model is more returns and more tokens, and both are counted in the task budget.
- **Plan ahead, run when free.** A task carries `not_before` and `priority`; approved tasks queue with their dependencies and any door (Slack, CLI, MCP) can add them. Quiet hours restrict messages and releases, not Builder work, so the factory runs day and night while the queue is not empty and the subscription share (`weekly_share`) leaves room for the owner's own use.
- **Nothing is lost silently.** The morning summary lists time spent waiting, tasks that ran on a fallback provider, and what is parked for a reset. A blocker message is sent only when the whole chain is out and the reset is more than 12 hours away.

### Consequences

- Good: no escalation for a normal limit; the factory resumes by itself after a restart or a reset.
- Good: the owner chooses the quality and cost trade-off with three settings (`max_wait`, `tier`, `blast radius` rule), not with code.
- Good: reuses the router, the chains, the pools and the handoff (a fresh session on another provider gets the handoff block).
- Bad: a fallback model may need more rounds; DeepSeek through `codex` is experimental until a live run passes (design section 12).
- Bad: the subscription reports no exact usage, so the reset time of a weekly limit is known only when the CLI says it.
- Bad: new tenant and provider schema fields (`schemas/` needs the owner's review) and one migration.

## Pros and Cons of the Options

### Wait and resume

- Good: simplest; no quality risk; no extra spend.
- Bad: a weekly limit can idle the factory for days with a full queue.

### Fallback only

- Good: no idle time.
- Bad: a weaker model works on high-risk tasks; pools drain on the first hiccup; short limits (minutes) switch models for no reason.

### Buy headroom

- Good: no new code.
- Bad: money is the only cap; Ultra credits are limited per month and do not cover every use (design section 18); it does not help when the vendor is down.

## Evidence

- Checked: `internal/dispatch/router.go` (`Park`, `NoteLimit` without callers, limits in memory), `internal/dispatch/build.go` (`building` escalates a failed job), `internal/jobsapi/server.go` (`rate_limit` stored as `failed`), `internal/harness/claude/claude.go` (`isLimit` reads message text only), design sections 7, 12, 18. Confidence: high.
- Checked: the stream-json `rate_limit_event` fields `status`, `resetsAt` (Unix ms), `rateLimitType` (`five_hour`, `seven_day`) and `utilization` as documented by an Elixir wrapper of the CLI ([ClaudeCode.Message.RateLimitEvent](https://claude-code.hexdocs.pm/ClaudeCode.Message.RateLimitEvent.html)), not by Anthropic. Confidence: medium; the harness test T8 must confirm the keys against the pinned `claude`.
- Checked: DeepSeek documents Codex with `wire_api = "responses"` and a fixed per-account concurrency limit that returns HTTP 429, so it behaves as a rate limit rather than a quota ([Codex integration](https://api-docs.deepseek.com/quick_start/agent_integrations/codex/), [rate limit](https://api-docs.deepseek.com/quick_start/rate_limit/)). The page was read in part; model names and prices change, so the provider stays experimental. Confidence: medium.
- Inferred: that `max_wait` of 2 hours and the 6-hour backoff cap are sensible defaults. Confidence: low; they are settings, and the first weeks of morning summaries will show the real distribution.
- Prior art not checked: LangGraph and Open SWE retry the same model with backoff and leave provider fallback to the application; this was not verified here.
