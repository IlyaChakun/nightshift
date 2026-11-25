---
status: accepted
date: 2026-11-25
---

# Offer terminal and MCP doors through an inbox chat adapter

## Context and Problem Statement

Slack carries the whole conversation: cards, the Lead dialog, approvals. A person who works alone and does not want Slack, or who prefers their own Claude (claude.ai project, Claude Code, Claude Desktop), has no way to see a card or press Approve. Only `task add` works without Slack. How do we add doors without a second interface codebase, a web UI or a new service?

## Considered Options

- An `inbox` chat adapter on the book, with CLI commands and an MCP server (official Go SDK) as thin doors over it
- `github-only` for solo tenants: cards as pull request comments
- A web UI of our own (Go, htmx)
- A bot per client (a separate integration for each of Slack, Claude and the terminal)

## Decision Outcome

Chosen option: "an `inbox` adapter with CLI and MCP doors", because the `Chat` interface already separates cards and chat events from Slack, the Claude client is itself the conversation partner, and the thread stays in the book so nothing is lost. The remote door for claude.ai is the same server over HTTPS, off by default; its token cannot merge, roll back or change settings.

### Consequences

- Good: one logic path for every door; each decision records its source.
- Good: no new service; stdio opens no port; the remote door listens on `127.0.0.1` and is exposed by a tunnel the person starts.
- Good: Slack is optional, so a solo user needs fewer accounts.
- Bad: one new dependency (the MCP SDK, approved by the owner on 2026-11-25) and a tenant schema value for the adapter.
- Bad: an internet-facing door, when enabled, makes a token the key to the owner's decisions; scopes and a short list of allowed tools limit the damage.
- Bad: what a claude.ai connector accepts for sign-in is not verified yet (design section 25).

## Pros and Cons of the Options

### `github-only` for solo tenants

- Good: nothing new to build.
- Bad: no place for the dialog before a pull request exists.

### A web UI of our own

- Good: the best experience.
- Bad: a new server, sign-in and frontend; the design defers it until a second person or the cloud.

### A bot per client

- Good: native feel in each client.
- Bad: three codebases to keep in step; the MCP server already reaches every Claude client.
