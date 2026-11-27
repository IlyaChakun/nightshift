# Knowledge plane

> Part of the [Nightshift design](../design.md). What the factory knows and finds: search over the book, project memory and the documents agents read. Memory of a tenant never leaks into another tenant.

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

**Two memory levels.**

| Level | Where | How it is read |
|---|---|---|
| Building the factory | `LEARNINGS.md` and the dev skills (`.agents/skills/`, for example `fix-ci`) in the `nightshift` repository | Changed by PR; read whole (under 100 lines), no search |
| The factory working for a tenant | Findings, notes and lessons per tenant in the book, indexed in `documents` | `ns search` |

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
