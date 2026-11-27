# internal/observability/

Observability plane (ADR 0026, design file `docs/design/11-observability.md`): what the factory measures about itself and reports, from the book and read-only repository health.

| Package | Owns |
|---|---|
| `summary` | the morning summary, the digest, `/ns status`, repository health and one task's timeline |

## Rules

- It reads state and decides nothing: no task moves, merges or messages start here; the caller sends what it builds.
- May import observability, data, customization, the contracts in `integration` and only `flow` from control; `internal/archtest` enforces it.
- A new book metric is a query in `internal/book/queries/` plus its wording here; its numbers stay in SQL.
