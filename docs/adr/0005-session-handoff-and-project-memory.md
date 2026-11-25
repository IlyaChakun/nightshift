---
status: accepted
date: 2026-11-25
---

# Keep a session handoff and a capped project memory page in the book

## Context and Problem Statement

A long task outlives one agent session: restarts, provider switches, context limits, and harnesses that never resume. Separately, the owner repeats the same facts to Lead ("tests run like this", "I prefer short answers"). The factory has findings, notes and `LEARNINGS.md`, but none of them carry the state of a running task or the current facts of a tenant. How do we cover both without a new store that becomes a dump?

## Considered Options

- A `handoff` block in role results, kept per (task, role), plus one capped project memory page per tenant with optimistic concurrency
- Rebuild context from the book on every restart and keep facts only as `notes` found by search
- Free-form agent memory files in the worktree or agent HOME, as harnesses do

## Decision Outcome

Chosen option: "a handoff block and a capped page", because both are small, validated and written by `nightshift`, so the rule "an agent never writes to the book" holds.

### Consequences

- Good: a restarted task continues from a short summary, checked against `events` and the worktree diff.
- Good: Lead stops asking for facts the owner already gave.
- Good: the page is capped, writes need the revision they read, and only the owner's messages and repository files are accepted as sources, so foreign text cannot poison it.
- Bad: two new schema fields (`handoff`, `memory[]`) and one table; they arrive with their phases and need the owner's review.
- Bad: the page is the only memory that enters prompts on its own; the cap and the nightly prune must hold or it turns into a dump.

## Pros and Cons of the Options

### Rebuild from the book only

- Good: nothing new to store.
- Bad: no summary of decisions made inside a session; facts found only by search are never in the prompt, so the owner is asked again.

### Free-form memory files

- Good: no code.
- Bad: lives in the worker, unvalidated, unbounded and exposed to foreign text; lost with the volume.
