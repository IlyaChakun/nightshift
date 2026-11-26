# Document length: how long each kind of document should be

## Summary

Outside guidance gives lengths only for design docs (1–3 pages for a mini design, 10–20 for a larger project), ADRs (one or two pages) and agent files (`CLAUDE.md` under 200 lines, `SKILL.md` under 500). For the other kinds we chose numbers so a section an agent loads fits one chunk. The budgets live in `skills/own/write-docs/SKILL.md` and are enforced on this repo by `scripts/doc-budget.sh`.

## Findings

| Kind | Guidance | Source, read 2026-11-26 |
|---|---|---|
| Design doc | "around 10-20ish pages" for a larger project; a "1-3 page 'mini design doc'" is fine | [Design Docs at Google](https://www.industrialempathy.com/posts/design-docs-at-google/) |
| ADR | "The whole document should be one or two pages long." | [Nygard](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions) |
| ADR | context "in free form using two to three sentences"; no length limit | [MADR](https://adr.github.io/madr/) |
| Postmortem | content named (impact, actions, root cause, follow-up), no length | [SRE book, ch. 15](https://sre.google/sre-book/postmortem-culture/) |
| `CLAUDE.md` | "target under 200 lines per CLAUDE.md file" | [Claude Code memory](https://code.claude.com/docs/en/memory) |
| `SKILL.md` | body "under 500 lines"; reference files over 100 lines get a table of contents | [Skill best practices](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices.md) |
| How-to, reference | four forms of documentation, no lengths | [Diátaxis](https://diataxis.fr/) |

Pages are converted at about 500 words per page (estimate, not measured). So a design doc max is 10,000 words, an ADR max 1,000.

## Fit (2026-11-26)

Seven files are over their max and listed in `scripts/doc-budget-baseline.txt`; the largest is `docs/design.md` with 35,370 words (about 58k tokens, `docs/research/token-economy.md`). Shrinking it to 10,000 words saves about 40k tokens on every full read (estimate). The work is stream D in `docs/plan/docs-budget/README.md`.

## Proposal

Budgets and the shrink pass in the `write-docs` skill for the factory and this repo; a CI check that fails on growth; then shrink the listed files, largest first.
