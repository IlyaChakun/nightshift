# A03 review — job result durability

Keep Go, Postgres and River. The audit distinguishes a committed state transition from applying an entire job result, but assigns runtime consequences from static write ordering. Start with a replay check on one logical result before introducing a new persistence model.

Checked revision: `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition | Evidence |
|---|---|---|
| A03.1 | Confirmed, details kept outside the repository | |
| State change and its history commit together | Confirmed | `internal/flow/move.go:42` opens the transaction, compares state/version/head and calls `record`; cancellation and the move event use that transaction at line 102. |
| Result consumption and state change can commit separately | Confirmed | `internal/dispatch/build.go:55` consumes before escalation. The success path reconciles completed jobs at line 126. Commit order alone establishes neither data loss nor its frequency. |
| Lease ownership and unique consumption already exist | Confirmed | `internal/book/queries/jobs.sql:40` checks ownership and expiry. Line 98 conditionally marks a terminal result consumed. Agent jobs use this table. River schedules the outbox flush at `internal/dispatch/river.go:63`. |
| Retry inventory covers documented side effects | Partly | `internal/dispatch/sideeffects_test.go:47` inventories syntax. The train journals rebase and CI at `internal/train/train.go:180`. The live GitHub check remains queued in `docs/plan/live-checks.md:120`. |

**Take — phases 3 and 5.** Extend X2 and R2 with one Postgres replay fixture covering repeated completion, competing writers, cancellation and restart. Compare result identity, one transition and one logical return against physical attempt usage. For a failing DB-local case, use one short transaction for consumption, mandatory facts, the existing move and dependent enqueue. Use errors for mandatory writes and keep optional telemetry separate. Use the existing lease predicate and journal before adding fencing or intermediate states.

**Take — phase 6.** Check adapter reconciliation after an ambiguous remote outcome. The outbox commits intent and delivers through a relay (`internal/dispatch/outbox.go:84`). As its pattern author puts it, “The Message relay might publish a message more than once.” River supports enqueueing “in a transaction with any other changes you’re making.” Both read 2026-11-26. [Transactional outbox](https://microservices.io/patterns/data/transactional-outbox.html) · [River](https://riverqueue.com/docs/transactional-enqueueing)

**Later — phase 5.** Model more failure permutations, measure relay lock time and add a separate policy projection only where the first check exposes a need. **Reject — phase 5.** A Temporal, DBOS or Restate migration on this evidence. ADR 0018 already records a Temporal recovery spike and named revisit triggers.

**Checked** — local `TestEverySideEffectIsListedAsRepeatSafe` passed. Read dispatch, book, flow and `docs/adr/0018-durable-execution-without-temporal.md` decision. **Inferred** — prioritization and transaction scope. **Unverified** — flow package tests (pinned rapid dependency unavailable), fault injection, provider idempotency, duplicate billing and production impact. **Confidence** — high for write order, medium for consequences.
