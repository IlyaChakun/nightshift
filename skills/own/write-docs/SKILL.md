---
name: write-docs
description: Write, edit or shrink any document (design doc, spec, ADR, runbook, postmortem, research note, plan, status, agent instructions, skill, note). Use before you write prose and before you report a doc as done; it loads the rules for that kind and ends with a mandatory shrink pass.
---

# Write docs

Every word costs a reader's attention and, for an agent, tokens on every later read. Write the answer first, in few words, under a heading a search would hit, then cut again.

## Steps

1. **Pick the kind** from the table and read its file in `types/`. One kind per document; a mix is two documents.
2. **Find the home.** Search for the fact first (`grep -rn`). If a document already says it, link to it and write nothing. One fact lives in one place.
3. **Write** with the shape rules below and the kind's sections.
4. **Shrink pass (mandatory).** Re-read the whole draft and make it shorter without losing a fact, a decision or a step: apply "Cut" below, merge repeated points, replace prose with a table where the reader looks things up, move detail to a link. Count words before and after (`wc -w`). Stop when removing any sentence would lose information.
5. **Check** (bottom of this file) and report the word count against the budget.

## Budgets

Words, because a line limit is gamed by long lines; about 500 words make one page (estimate). "Max" is the hard limit; a document over it is split by topic, never compressed into dense text.

| Kind | File | Min | Typical | Max |
|---|---|---|---|---|
| Design doc of a whole system | `types/design.md` | 1,000 | 5,000 | 10,000 |
| Spec or mini design | `types/design.md` | 300 | 800 | 1,500 |
| ADR | `types/adr.md` | 150 | 400 | 1,000 |
| Postmortem or retro | `types/postmortem.md` | 200 | 500 | 1,000 |
| Runbook, setup or how-to file | `types/runbook.md` | | 150 per task | 4,000 |
| Research note | `types/research.md` | 300 | 800 | 1,500 |
| Plan item | `types/plan.md` | 50 | 200 | 800 |
| Status page | `types/plan.md` | | 300 | 600 |
| Agent instructions (`AGENTS.md`, `CLAUDE.md`) | `types/instructions.md` | | 800 | 1,800 and 200 lines |
| Skill (`SKILL.md`) | `types/instructions.md` | | 600 | 1,000 and 100 lines |
| Reference page (secrets, decisions, glossary) | `types/runbook.md` | | | 3,000 |
| Note or memory entry | `types/note.md` | | 100 | 300 |

Below "Min" a document usually lacks context or a required section; add it or make the text a section of another document. The sources of these numbers are in the kind files.

## Shape

- **Title and summary.** The H1 uses the words a reader would search for. Under it, at most three sentences: what this is, the outcome or rule, who needs it. A reader who stops there knows the answer.
- **Headings** in sentence case name the thing (`Restore a backup`, not `Overview`) and are unique in the file. Each section stands alone: agents read a chunk, not the file.
- **Exact names in backticks**, copied from the source: commands, files, keys, env vars, error texts. One term per concept.
- **Conditions before instructions**; active voice, present tense, no history ("was/now", "v2", "replaces"). Absolute dates (`2026-11-26`).
- **Tables** for lookup, **numbered lists** for ordered steps, **prose** for reasons. A diagram only where flow is the point, and it must render.
- **Facts about the outside world** carry a link and a date, or are marked **unverified**.
- Over 100 lines: a table of contents under the summary.

## Cut

Delete a sentence if removing it changes no decision or action:

- filler ("in order to", "it is important to note", "basically", "currently", "simply", "as mentioned above", "please note");
- restating the heading, announcing what comes next, summing up what was just said;
- hedging without information; say what is unknown and how to check;
- process narration ("first we tried"): git and ADRs keep history;
- a second copy of a fact that lives elsewhere: link it.

## Check before you finish

1. **Summary test**: read only the title and summary. Is the outcome clear?
2. **Search test**: three words a newcomer would type; does `grep -rni` find the right heading?
3. **Cut test**: did the shrink pass run, with the word counts before and after?
4. **Budget**: `wc -w <file>` is under the kind's max.
5. **Truth**: every command, path and key named exists; links open; diagrams render; YAML parses.

Say in your report which of these you ran.
