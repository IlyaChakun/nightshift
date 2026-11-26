# Stream — short documents and their budgets

Owned by the session working on this stream; another session asks it and does not edit this file. Rules and limits: `docs/plan.md`. Source: the owner's request of 2026-11-26 (keep repo docs short, a skill per kind, a CI check).

Done when: every file in `scripts/doc-budget-baseline.txt` fits its kind's max and the file is empty, the check runs in CI, and Lead and Builder load `own/write-docs`.

## Status

- Last done: D1; D2 sections 21, 12, 20 and 15.
- Next: D2, the largest sections of `docs/design.md` first; section 21 is already cut (1,532 to 379 words, in D1).
- Open: none.

## Items

- [x] D1. `feat(docs): write-docs budgets per kind, shrink pass, CI check` — `skills/own/write-docs` with `types/`, `.agents/skills/write-docs` as its repo layer, `scripts/doc-budget.sh` with a baseline that may only shrink, `docs/research/doc-length.md`; skill on Lead and Builder
- [ ] D2. `docs(design): shrink docs/design.md to 10,000 words` — several PRs by section, each with `docs/design.html` and the `design-sync` hashes; meaning kept, detail moved to ADRs and the runbook by link
- [ ] D3. `docs(runbook): shrink docs/runbook.md to 4,000 words` — split by topic if cutting is not enough
- [ ] D4. `docs: shrink the ADRs, retro, research note and skill in the baseline` — one PR
