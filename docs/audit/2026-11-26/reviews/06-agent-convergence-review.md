# A06 review — finding identity and repair progress

Make finding delivery and verified resolution distinguishable before using either to score progress. The audit identifies two writers and conflicting interpretations of the same status. Its decoder demonstration supports a field-mapping diagnosis, while actual duplicate counts and benefits from adaptive retries remain unmeasured.

Checked revision: `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition | Evidence |
|---|---|---|
| Supervisor and dispatcher already bound attempts | Confirmed | `internal/harness/supervise/supervise.go:54` compares consecutive tool calls. Lines 79 and 186 inspect reported usage. `internal/dispatch/verdict.go:33` bounds review returns. These mechanisms do not measure semantic progress. |
| A returned finding has inconsistent readers | Confirmed statically | `internal/jobsapi/memory.go:28` tags the schema fields. `internal/dispatch/verify.go:112` uses untagged `RuleID` and `LayerCaught` for the snake-case fields and inserts on a returned verdict. It hashes the path without storing it in the second row. |
| A returned verdict can create duplicate findings | Partly | `internal/jobsapi/server.go:403` invokes validated ingestion. `internal/book/migrations/00005_findings_notes.sql:3` permits repeated fingerprints. Static paths can both insert for a returned verdict. No database row count observed. |
| Fixed means verified resolved | Confirmed mismatch | `internal/dispatch/build.go:283` sets `fixed` after enqueueing the next job. `internal/check/sarif.go:143` prefers tool fingerprints and otherwise includes message text, so rewording can change identity. |
| Continuation can use an existing PR tip | Confirmed | `internal/dispatch/build.go:364` selects a PR tip or base. Handoffs are capped at `internal/dispatch/handoff.go:19`. `internal/dispatch/research.go:32` bounds eligible returns to Lead. |

**Take — phases 5–6.** Add a Postgres contract check for one returned schema-valid finding across completion, dispatch and replay. Assert identity, rule, layer, location and count. Consolidate ingestion using the validated record. Track delivery separately from closure and close only on a comparable independent result or authorized adjudication. Give Builder a redacted summary of remaining obligations.

**Later — phase 7.** Compare a deterministic escalation policy against baseline on fixed tasks with equal budgets. Preserve revision, suite, attempt, cost and per-criterion outcomes. Test genuine progress, alternating patches, regression, incomplete checks and transient failure. Keep memory tenant/role scoped with provenance. Consider checkpoint recovery, context refresh, fallback or bounded replanning only if fewer retries preserve independently verified acceptance.

**Reject — phase 7.** Raw finding count as a convergence score, unbounded model diagnosis and automatic promotion of model memory. Huang et al. report that in reasoning tasks models “struggle to self-correct their responses without external feedback”. Self-Refine and Reflexion report benefits on their own tasks. None measures Nightshift. Read 2026-11-26. [ICLR](https://proceedings.iclr.cc/paper_files/paper/2024/hash/8b4add8b0aa8749d80a34ca5d941c355-Abstract-Conference.html) · [Self-Refine](https://proceedings.neurips.cc/paper_files/paper/2023/hash/91edff07232fb1b55a505a9e9f6c0ff3-Abstract-Conference.html) · [Reflexion](https://papers.neurips.cc/paper_files/paper/2023/hash/1b44b878bb782e6954cd888628510e90-Abstract-Conference.html)

**Checked** — local `TestVariedCallsAreNotALoop` and `TestHandoffText` passed. Read ingestion, dispatch, schema, migration, supervisor, handoff and ADR 0019. **Inferred** — effect on scoring. **Unverified** — database replay, live spend, false stagnation stops and comparative outcomes. **Confidence** — high for field/status semantics, medium for runtime effect.
