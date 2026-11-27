---
status: accepted
date: 2026-11-24
---

# Use one Postgres for the book, the queue and search

## Context and Problem Statement

The factory needs a source of truth for tasks and events, a job queue with retries and cron, and full-text (later vector) search. It runs on one machine and is maintained by one owner. How many stateful services do we run?

## Considered Options

- Postgres 17 + pgvector with River as the queue
- Postgres + Temporal for workflows
- Postgres + Redis or Kafka for the queue, plus a separate vector DB

## Decision Outcome

Chosen option: "Postgres 17 + pgvector with River", because one database gives transactional enqueueing together with state changes, one backup and one thing to operate.

### Consequences

- Good: Postgres lets a state change and its follow-up records share one transaction. See [design section 3](../design/07-data-foundation.md#3-architecture-of-the-first-version) for the worker jobs and outbox paths.
- Good: one `pg_dump` and restic backup covers everything; Grafana reads the book directly.
- River runs inside `nightshift` and schedules outbox delivery. The jobs API leases agent work from `book.jobs`, so the worker needs only HTTP access.
- Bad: if load ever outgrows one Postgres, the queue moves first. Not expected for one owner.

## Pros and Cons of the Options

### Temporal

- Good: durable workflows and replay out of the box.
- Bad: a cluster of its own services and a second persistence layer; too much for one host.

### Redis or Kafka plus a vector DB

- Good: well-known queue semantics.
- Bad: three stateful services instead of one and no transaction across queue and book.
