# Plan

Phases and their "Done" criteria are defined in `docs/design.md` (section 22).
This file tracks the current phase only. Limit: 150 lines.
Rules: one item = one PR, under ~400 changed lines. Tick an item in the PR that completes it.
Add an item only if it follows from the design and the owner agreed.

Phase 0 (skeleton and contracts) is done: PRs #1–#13.

## Current phase: 1 — book and core

- [x] 1. `feat(book): add schema and goose migrations` — tasks (with `repo`), append-only `events`
  (a trigger rejects UPDATE and DELETE), decisions, budgets, outbox; roles `nightshift`,
  `grafana_ro` and a migration owner; T3 on testcontainers Postgres
- [x] 2. `feat(book): add sqlc queries` — task CRUD, event append, outbox enqueue and claim; T3
- [ ] 3. `feat(flow): add task state machine with CAS transitions` — transition table from design
  section 7, CAS on (state, version, `head_sha`), idempotent decisions, stricter decision wins,
  task budget; T1 tables and T2 rapid properties
- [ ] 4. `feat(config): load the instance and tenant pack` — files + schema validation, compose
  secrets read from files, never from env; T1 on `example-solo` and `example-team`
- [ ] 5. `feat(dispatch): add River queue and outbox worker` — secret masking before send,
  retries with backoff, idempotent delivery; T1 for masking, T3 for the worker
- [ ] 6. `feat(adapters): add Slack Socket Mode chat and its twin` — replies in the task thread,
  commands only from `owner` and `approvers`, link unfurling off; twin in `adapters/chat/fake`
- [ ] 7. `feat(serve): wire nightshift serve` — slog JSON, OpenTelemetry traces, health endpoint,
  graceful shutdown, state survives a restart; T4-lite: Slack twin message → reply in thread
- [ ] 8. `feat(deploy): add compose for postgres and nightshift with backups` — `pg_dump` hourly,
  restic daily, weekly restore test, restore steps in `docs/runbook.md`
- [ ] 9. Done check: `docker compose up` on the home machine, the bot answers in a Slack thread of
  the first tenant, a restart keeps state, restore from a copy works by the runbook

## Phase 1 done when

- After `docker compose up` the bot answers in a Slack thread of the first tenant.
- State survives a restart; restoring the book from a copy works by `docs/runbook.md`.
- Tests T1–T3 are green without network.

## Next phases

2 — runner, harnesses, isolation · 3 — Builder, checks, gates.
Phases 4–10 start only when the previous ones hurt without them (design section 22).
