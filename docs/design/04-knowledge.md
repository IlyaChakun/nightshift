# Knowledge plane

> Part of the [Nightshift design](../design.md). What the factory knows and finds: search over the book, project memory and the documents agents read (section 13), and the scopes that knowledge lives in, from the factory down to one task (section 28). Memory of a tenant never leaks into another tenant.

## 13. Search

**Over code: we build nothing.** Harnesses search on their own: grep, symbols, reading files. Our part is a short `AGENTS.md` and a clear `docs/` layout.

**Over knowledge: one table in the same Postgres:**

```sql
documents(id, tenant_id, kind, source_url, title, body, quoted_at, visibility,
          status,              -- active | proposed | superseded
          superseded_by,       -- id of the document that replaced this one
          provenance jsonb,    -- task, role, model, job for documents written from agent output
          tsv tsvector,        -- full text, russian and english configurations
          embedding vector,    -- pgvector, nullable: empty until an embedder is configured
          embedding_model,     -- model id the embedding was made with
          created_at)
-- kind: decision | adr | prd | standard | learning | finding | tracker_issue | pr | commit | chat_thread | incident
--       | task_outcome | note
-- visibility: all | lead_verifier (Builder does not see hidden scenarios)
```

**Memory types map to what exists; there is no separate "agent brain".** Working memory is the harness session and its handoff (section 12); facts the owner would otherwise be asked again live on the project memory page (below); episodic memory is the book (`events`, findings, reviews), transcripts in `artifacts` and `task_outcome` documents; semantic memory is ADRs, PRDs, `docs/` and decisions; procedural memory is standards, skills, `AGENTS.md` and `LEARNINGS.md`, changed only by learn through the fix ladder (section 10).

**Scopes.** Factory knowledge and tenant knowledge are separate scopes, each with its own home and writer (section 28).

An approved tenant lesson becomes a lint rule, a test or an entry in the tenant repository (`LEARNINGS.md`, a skill), so the team sees it in review; its `learning` document is an index rebuilt from git. Search finds similar cases; it is never the only home of a lesson.

- **Task outcome.** When a task reaches `released`, `handed_off`, `canceled` or `escalated`, the runner writes a `task_outcome` document from the PR summary (section 8) and the book: what was done, what failed and why, PR and transcript links. Lead searches them before planning.
- **Agent notes.** Agents never write to the book. A role result may carry `notes[]` (kind, text, source), validated against `schemas/`; `nightshift` stores each as `kind=note`, `status=proposed` with provenance. Notes are searchable and marked "unverified" but never put into a prompt automatically; learn may promote one through the usual approvals. This limits memory poisoning through foreign text ([MINJA](https://arxiv.org/abs/2503.03704)).
- **Project memory.** One page per tenant in the book for facts the owner would otherwise be asked again (how tests run on this machine, the preview environment, "short answers"). It is the only memory put into prompts on its own: whole, as data, into Lead's and Builder's context, which is safe because it is small, capped and strictly written.
  - **Shape and cap.** One-line entries (`key`, `text`, `source`, `expires_at`), at most 40 entries and 4 KB, with a `revision`. A write over the cap is refused ("page full: merge or drop an entry"); nothing is evicted silently.
  - **Writes.** Only through `nightshift`, read-then-write: a write names the `revision` it read and a stale one is refused (compare-and-swap, as in section 7). The owner writes with `/ns remember <text>`; Lead proposes `memory[]` entries, accepted only when `source` is an owner message or a tenant repository file. Builder and Verifier cannot write it, and text from tickets, the web or previews never reaches it (section 10, item 8).
  - **Pruning.** Nightly, without a model: duplicates, entries whose fact now lives in the repository (with a link), and entries past `expires_at` (90 days unless pinned). Each drop is an `events` row; `/ns memory` shows the page and the drops.
  - **Not a second `LEARNINGS.md`.** Stable rules go to the repository through the fix ladder; the page holds no findings, lessons or task history.
- **Superseded decisions** set `superseded_by`; search returns them only on request, linked to the replacement.
- **Indexer:** a River job reindexes changed documents and standards after a merge to `main`, and a decision after a thread answer.
- **Who searches:** Lead before asking the owner and in #support (duplicates), learn (does the rule exist?), any role through `ns search` within its visibility.
- **One retrieval interface, hybrid-ready.** Full text (`tsvector` + `ts_rank`) always; when `search.embedder` is set, vector search too, fused with Reciprocal Rank Fusion (a formula, no model). pgvector and the nullable `embedding` column exist from the first migration, so without an embedder nothing extra runs. The vector half adds synonyms ("timeout" and "deadline exceeded").
- **Search eval.** From phase 5 the book collects up to 20 real questions search should have answered, with the expected document; the owner sets `search.embedder` once hybrid search puts it in the top three more often than full text.
- **What is embedded:** `decision`, `adr`, `prd`, `learning`, `task_outcome`, `incident`, `tracker_issue`, split by headings; not chat threads, findings, transcripts or code.
- **Embedding model.** An `Embedder` interface in `internal/harness/`, nothing configured by default: a local multilingual model (such as bge-m3 through Ollama or ONNX, in an optional compose profile) keeps texts on the machine; an API provider means egress and a key, set by the instance or tenant pack. Each row stores `embedding_model`; a model change means a cheap reindex from git and the book.
- Every result carries a quote, source and date. Slack Free shows 90 days, so what matters in threads is copied into the book at once.

## 28. Knowledge scopes: factory and tenant memory

**Scope and type are two axes.** Section 13 names the memory types (working, episodic, semantic, procedural). The scope says whose knowledge it is and who may change it. Any type can live at any scope; a reader gets only the scopes and visibility its role and tenant allow, and `nightshift` applies that filter, never the prompt.

| Scope | What it holds | Where it lives | Who changes it |
|---|---|---|---|
| Factory | Rules for every tenant: roles, factory skills, industry standards, schemas, gates; the lessons of building Nightshift itself | The `nightshift` repository: `roles/`, `skills/`, `standards/`, `schemas/`, `LEARNINGS.md`, `.agents/skills/` | A PR to `nightshift` through its own gates; a tenant gets it with a release |
| Instance | Trust domain, providers, egress | `instance.yaml` (section 17) | The operator |
| Tenant | Findings, notes, lessons, task outcomes, the project memory page | The book, keyed by `tenant`, indexed in `documents` | Only `nightshift` (section 13) |
| Project | Instructions, project standards and skills, ADRs, PRDs, `LEARNINGS.md` | The project repository (`.nightshift/`, `AGENTS.md`, `docs/`) | A PR in that repository |
| Task | Working memory | The harness session and its handoff; transcripts in `artifacts` | The harness; at the end only the `task_outcome` document stays |

**Rules.**

- **The instance is the trust boundary.** Tenants of one instance are kept apart by the `tenant` key in every query and by policy, not physically (section 17, "Several tenants in one instance"). A claim of stronger isolation needs a negative test first (stream `docs/plan/architecture/`, A7).
- **Factory knowledge flows down, tenant knowledge does not flow up.** Nothing a tenant wrote reaches the factory scope or another tenant on its own. Promotion is a person's PR to `nightshift` with a tenant-neutral rule, check or skill that carries no tenant text, reviewed like any change. Across instances only the counts-only operator report leaves, and only by hand (ADR 0016).
- **A new kind of memory never widens a right.** It is read within the reader's visibility and written through `nightshift`, like every document today.
- **Ownership.** Knowledge owns the meaning: kinds, scopes, retrieval and what goes into a role's context. Data foundation owns the records, provenance columns and retention (section 14). Assurance decides whether evidence is enough for a gate (section 8); a retrieved text never authorizes an action.

**Direction.** The components below are proposals until the owner adopts them; each links to its research. No graph database, GraphRAG or new service is added only to classify knowledge.

| Component | Today | Direction |
|---|---|---|
| Memory management | The types and the project memory page (section 13) | Write policy and life cycle per type |
| Retrieval | Full text, hybrid-ready (section 13) | Graph retrieval only after a measured need |
| Context engineering | `nightshift` puts the project memory page into Lead's and Builder's context; search on request | A context budget per role ([`docs/research/token-economy.md`](../research/token-economy.md)) |
| Product intelligence | PRDs and decisions in `documents`, the project memory page | A tenant-scoped product charter that separates evidence from hypotheses ([PR #340](https://github.com/IlyaChakun/nightshift/pull/340)) |
| Codebase intelligence | The harness searches the code; `AGENTS.md` | Revision-bound change impact analysis ([PR #344](https://github.com/IlyaChakun/nightshift/pull/344)) |
| Provenance | `provenance`, `status` and `superseded_by` on each document | Freshness, confidence and lineage ([PR #369](https://github.com/IlyaChakun/nightshift/pull/369)) |
| Cross-tenant learning | A person's PR to `nightshift` | Assisted promotion of tenant-neutral artifacts ([PR #351](https://github.com/IlyaChakun/nightshift/pull/351)) |
