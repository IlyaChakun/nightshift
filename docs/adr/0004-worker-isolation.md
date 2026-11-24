---
status: accepted
date: 2026-11-24
---

# Isolate agents in a worker container without database or tokens

## Context and Problem Statement

Agents run untrusted project code and read foreign text. They must not reach secrets, the book, the home network or arbitrary hosts. How do we isolate them on one machine without Kubernetes?

## Considered Options

- One job per hardened worker container: no Postgres access, jobs API with per-job tokens, agent under a separate uid, outbound only through Smokescreen by allowlist
- Docker-in-Docker sandboxes per job
- Run agents directly in the `nightshift` container

## Decision Outcome

Chosen option: "one job per hardened worker container", because it gives a hard boundary with plain compose: read-only rootfs, `cap_drop: ALL` except `SETUID` and `SETGID` for the runner, an internal network, and egress only by allowlist.

### Consequences

- Good: the agent cannot read the book, other jobs or `/run/secrets`; T6 escape tests check this.
- Good: `nightshift` applies patches in its own bare copy with hooks off and pushes with its own token.
- Bad: harness credentials still live in the worker; mitigated by egress, a dedicated token, rotation and credits without auto-reload.
- Bad: preview through `docker compose` is not possible from the worker; previews use project scripts or external environments.

## Pros and Cons of the Options

### Docker-in-Docker

- Good: per-job sandboxes.
- Bad: needs a privileged daemon or socket, which breaks the boundary.

### Agents in `nightshift`

- Good: simplest.
- Bad: agents would share a process with every secret and the database.
