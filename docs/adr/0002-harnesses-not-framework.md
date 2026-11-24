---
status: accepted
date: 2026-11-24
---

# Run roles through coding-agent harnesses, not an agent framework

## Context and Problem Statement

Lead, Builder and Verifier need a strong agent loop: tools, file edits, skills, sessions. We must not depend on one vendor, and the Claude subscription works only through the native `claude` CLI. Do we build roles on an agent framework or drive existing harnesses?

## Considered Options

- A `Harness` interface with adapters over CLIs (`claude`, later `copilot`, `codex`, `opencode`, `acpx`), plus an `api` adapter for tool-less calls and `fake` for tests
- An agent framework (Deep Agents, LangGraph) as the role runtime
- `acpx` as the only adapter for all agents

## Decision Outcome

Chosen option: "a `Harness` interface with CLI adapters", because harnesses already carry the best agent loops and skills, a provider becomes one line of config, and the subscription stays inside the unmodified `claude`.

### Consequences

- Good: changing vendor or model is a config change; the router can fall back across providers.
- Good: the `fake` adapter makes the whole factory testable without models.
- Bad: each CLI has its own flags and output; adapters must normalize them and the T8 contract test must track CLI versions.
- Bad: some CLIs cannot validate output against a schema; the runner validates every result itself.

## Pros and Cons of the Options

### Agent framework

- Good: full control of the loop.
- Bad: we would rebuild what harnesses already do, and the subscription cannot be used there.

### acpx only

- Good: one protocol for all agents.
- Bad: pre-1.0, and the Claude subscription works only in the native `claude`.
