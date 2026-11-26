---
name: write-docs
description: Write, edit or shrink any document in this repo (docs/, ADR, runbook, status, plan, README, skill, PR text) so a person and an agent can read it fast and find it by search. Use before you write or change prose, and before you report a doc as done.
---

# Write docs in this repo

The rules, budgets, kinds and the mandatory shrink pass are the factory's own skill: read `skills/own/write-docs/SKILL.md` and the file for your kind in `skills/own/write-docs/types/`, and follow them. This file adds only what is specific to this repository.

## Budgets enforced here

`scripts/doc-budget.sh` runs in `make verify` (and so in CI) and counts words (not lines) and fails when a file in this table is over its max:

| Path | Kind | Max words |
|---|---|---|
| `docs/design.md` | design doc | 10,000 |
| `docs/adr/*.md` | ADR | 1,000 |
| `docs/retro/*.md` | postmortem or retro | 1,000 |
| `docs/research/*.md` | research note | 1,500 |
| `docs/runbook.md`, `docs/setup.md` | runbook | 4,000 |
| `docs/secrets.md`, `docs/decisions.md`, `docs/prior-art.md` | reference page | 3,000 |
| `docs/live-checks/*.md` | plan item | 800 |
| `.agents/skills/*/SKILL.md`, `skills/own/*/SKILL.md` | skill | 1,000 |

Not in the script: a PR description keeps what, why and risks to 5 lines, then the blocks `AGENTS.md` requires; the line limits of skills and instructions are for you.

The handoff files keep their own line and word limits in `scripts/repo-check.sh` (`docs/status.md`, `docs/plan.md`, `docs/plan/`, `AGENTS.md`).

A file that was over its max when the check was added is listed in `scripts/doc-budget-baseline.txt` with its word count then. It may not grow; when it shrinks, lower its number to the new count in the same PR (the check fails otherwise), and delete the line once the file fits. A PR that adds words to such a file cuts as many in it. Raising a number is weakening a protection (`AGENTS.md`, "Ask before you do it").

## Repo rules

- The facts and limits of each handoff file: `docs/AGENTS.md` and `docs/plan.md`.
- A design change also updates `docs/design.html` and the hashes: skill `change-design`.
- `make docs` runs markdownlint; `cmd/nightshift/promises_test.go` fails on a command, path or key that does not exist.
- In the PR, report the word counts before and after the shrink pass.
