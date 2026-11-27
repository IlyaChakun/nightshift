# Stream — short documents and their budgets

Owned by the session working on this stream; another session asks it and does not edit this file. Rules and limits: `docs/plan.md`. Source: the owner's request of 2026-11-26 (keep repo docs short, a skill per kind, a CI check).

Done when: every file in `scripts/doc-budget-baseline.txt` fits its kind's max and the file is empty, the check runs in CI, and Lead and Builder load `own/write-docs`.

## Status

- Last done: D2 and D2b: the design is split into the overview `docs/design.md` (5,922 words) and ten plane files under `docs/design/`, each under 10,000 words; planes in ADR 0026.
- Next: D2c with the owner (package names and architecture tests); D3 runbook split by topic.
- Open: none.

## Items

- [x] D1. `feat(docs): write-docs budgets per kind, shrink pass, CI check` — `skills/own/write-docs` with `types/`, `.agents/skills/write-docs` as its repo layer, `scripts/doc-budget.sh` with a baseline that may only shrink, `docs/research/doc-length.md`; skill on Lead and Builder
- [x] D2. `docs(design): shrink docs/design.md to 10,000 words` — several PRs by section, each with `docs/design.html` and the `design-sync` hashes; meaning kept, detail moved to ADRs and the runbook by link
- [x] D2b. `feat(docs): split the design into layer files` — owner, 2026-11-27: the design is layered like the factory, `docs/design.md` a high-level overview within its budget, one `docs/design/<layer>.md` per layer. First the tooling (`design-sync.sh`, the design-term check, `doc-budget.sh`, `design_test.go` read every file); the split itself after the layer list is settled in the owner's layers thread; planes and rules in ADR 0026
- [ ] D2c. `test(arch): enforce the plane boundaries of ADR 0026` — owner, 2026-11-27: after the split, agree package names per plane with the owner, audit today's imports, then add architecture tests (allowed imports between planes, no adapter import in the core) before any package moves
- [ ] D2d. Proposed (owner's review of #417, 2026-11-27), waits for the owner's OK: `docs/design/11-observability.md` (signals, metric definitions, read models, health; section 15 keeps its number), a knowledge file covering memory types and scopes, retrieval, context and provenance (research PRs stay proposals), `docs/design/12-capabilities.md` (how a stack, skills, checks and deploy targets make one engineering capability, with #352), and the overview cut toward ~2,000 words (estimate)
- [ ] D3. `docs(runbook): shrink docs/runbook.md to 4,000 words` — split by topic if cutting is not enough
- [ ] D4. `docs: shrink the ADRs, retro, research note and skill in the baseline` — one PR
