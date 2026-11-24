---
name: add-integration-adapter
description: Add or extend an adapter for an external system (tracker, chat, code host, docs, checks) and its twin. Use when the core must reach a new service or a new call of an existing one.
---

# Add an integration adapter

The core knows interfaces, not services. `internal/integration/` holds the interfaces; concrete services live only in `internal/adapters/<kind>/<name>/` and are chosen by name from `tenant.yaml` (design section 21).

1. **Interface first.** Add or extend the interface in `internal/integration/` only when a second caller needs it. Optional abilities are separate small interfaces (see `Reactor` in `internal/adapters/chat.go`).
2. **No names in the core.** `scripts/repo-check.sh` fails when a service name appears under `internal/` outside `internal/adapters/` and `testdata/`. Put the name in the adapter or a pack, never in a condition in the core.
3. **Write the twin before the adapter.** `<name>/fake` is an `httptest` server with an in-memory state and scripted faults (see `internal/adapters/github/fake`). Tests talk to the twin, never to the real service.
4. **Failure paths are tests.** 429, 5xx, timeouts and a duplicate delivery get a T5 case; every side effect is idempotent and goes through the outbox.
5. **Secrets as files.** The adapter reads its token from a file path in config, never from `os.Getenv`; the worker never gets these tokens.
6. **Check `internal/<pkg>/AGENTS.md`** of the caller (dispatch, train, gitops) for its traps, and update that note when the contract changes.

Egress hosts, secrets and permissions are gated: batch them as one owner question.
