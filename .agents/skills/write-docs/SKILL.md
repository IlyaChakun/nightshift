---
name: write-docs
description: Write or edit any document in this repo (docs/, ADR, runbook, status, plan, README, skill, PR text) so a person and an agent can read it fast and find it by search. Use before you write or change prose, and before you report a doc as done.
---

# Write docs

A reader (human or agent) has a question and little time. Every word costs their attention and an agent's tokens. Write the answer first, in few words, under a heading a search would hit.

## Before you write

1. **One job per document.** Pick one kind: how-to (steps for a goal), reference (facts to look up), explanation (why), status or plan (what is next), decision (ADR). Do not mix kinds in one section. [Diátaxis](https://diataxis.fr/) names the four forms.
2. **Find the home.** Search the repo for the fact (`grep -rn`). If a doc already says it, link to it and write nothing. One fact lives in one place; copies drift.

## Shape

- **Title and summary.** The H1 uses the words a reader would search for. Under it, a summary of at most three sentences: what this is, the outcome or rule, who needs it and when. A reader who stops there must still know the answer. This is also the [llms.txt](https://llmstxt.org/) pattern: a short map first, detail behind links.
- **Headings** are sentence case, name the thing (`Restore a backup`, not `Overview`), and are unique in the file. Each section stands alone, because agents read a chunk, not the file.
- **Exact names in backticks**: commands, files, config keys, env vars, error texts. Copy them from the source; one term per concept, everywhere. A `grep` for the name must find the section.
- **Conditions before instructions**: "If the build fails, run `make verify`", not the reverse. Active voice, present tense, no history ("was/now", "v2", "replaces").
- **Tables** for lookup, **numbered lists** for ordered steps, **prose** for reasons. A diagram only where flow or structure is the point, and it must render.
- **Facts about the outside world** carry a link and a date, or are marked **unverified** (AGENTS.md, "Evidence").
- **Absolute dates** (`2026-11-26`), never "yesterday" or "recently".
- Over 100 lines: add a table of contents under the summary ([Anthropic skill guide](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices.md)). Split a file by topic before it grows past a few hundred lines.

## Cut

Delete a sentence if removing it changes no decision or action. Common cuts:

- Filler: "in order to", "it is important to note that", "basically", "currently", "simply", "as mentioned above", "we should", "please note".
- Restating the heading, announcing what comes next, summing up what was just said.
- Hedging that carries no information ("may possibly"). If you are unsure, say what is unknown and how to check.
- Process narration ("first we tried"). Git and ADRs keep history.
- Long lines used to stay under a line limit. The limits count words too (`scripts/repo-check.sh`).

## Budgets

| Document | Limit |
|---|---|
| `docs/status.md` | 40 lines and 600 words |
| `docs/plan.md` | 60 lines and 800 words |
| `docs/plan/*.md` | 200 lines and 4000 words each |
| ADR | one page: about 400 words |
| Runbook or setup section | one task, about 150 words plus commands |
| `SKILL.md` | under 100 lines (hard limit 500) |
| PR description | what, why and risks in 5 lines, then the required blocks |

`scripts/repo-check.sh` enforces the limits of the handoff files; the others are for you.

## Check before you finish

1. **Summary test**: read only the title and summary. Do you know the outcome?
2. **Search test**: list three words a newcomer would type. Does `grep -rni` find the right heading?
3. **Cut test**: remove each sentence in turn. Did anything change? If not, leave it out.
4. **Count**: `wc -w <file>` against the budget.
5. **Truth**: every command, path and key you named exists (run it or `grep` it); links open; mermaid renders; YAML parses.
6. `make verify` is green.

Say in your report which of these you ran.
