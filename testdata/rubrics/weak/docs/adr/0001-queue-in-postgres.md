---
status: accepted
---

# Keep the job queue in Postgres

## Context and Problem Statement

Example-solo needs a job queue for nightly work.

## Considered Options

- Postgres-backed queue
- A separate message broker

## Decision Outcome

Postgres-backed queue: one database to run and back up. A broker is rejected because it adds a service. Consequence: queue throughput is bounded by the database.
