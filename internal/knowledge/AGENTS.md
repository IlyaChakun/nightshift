# internal/knowledge/

Knowledge plane (ADR 0026, design file `docs/design/04-knowledge.md`): what the factory knows and finds.

| Package | Owns |
|---|---|
| `search` | full-text and optional vector search over `book.documents` |
| `memory` | the capped project memory page, written by compare-and-swap |

## Rules

- May import only knowledge, data (`book`) and customization packages; `internal/testing/archtest` enforces it.
- Every read filters by tenant (search also by visibility): one tenant's knowledge never reaches another.
- Knowledge owns meaning; the rows belong to the data plane, the evidence to assurance.
- A new embedder or retrieval source implements the interface its caller owns; no provider SDK here.
