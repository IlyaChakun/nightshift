# docs/

Write every document with `.agents/skills/write-docs`: summary first, one fact in one place, no filler.

## Which document answers what

| File | Question |
|---|---|
| `design.md`, `design/NN-<plane>.md` | what the factory is and why: the overview and plane map, then one file per plane (ADR 0026), with unique section numbers (the only living design; `design.html` is its view) |
| `adr/NNNN-*.md` | why one decision was taken (MADR 4.0, at least two options) |
| `plan.md` | the owner's page: phases, states, links (limit 60 lines, 800 words) |
| `plan/<phase or stream>/README.md` | one phase or stream each: status and items; its session owns it (200 lines, 4000 words) |
| `plan/<phase or stream>/<id>-<topic>.md` | one item proposed by another session; a new file, so no conflict (80 lines, 800 words) |
| `plan/live-checks.md` | which merged work still owes a check on a real stack, and its result |
| `status.md` | what holds across phases (limit 40 lines, 600 words; rewrite, never append) |
| `decisions.md` | what the owner still has to decide |
| `runbook.md`, `setup.md`, `secrets.md` | how a person runs, installs and configures it |
| `prior-art.md`, `research/` | what other systems do |

## Rules

- Word budgets per kind of document: `scripts/doc-budget.sh` in `make verify`. A file in `scripts/doc-budget-baseline.txt` may not grow: add words only by cutting as many.
- Write as current: no "v2", "replaces", "was/now".
- A design change updates `design.html` in the same PR, then `scripts/design-sync.sh --update` after reading the changed sections. Use `.agents/skills/change-design`.
- An architecture change needs an ADR in the same PR.
- Check that mermaid diagrams render and YAML examples parse. `make docs` runs markdownlint.
- Facts about the outside world carry a link and a date, or are marked unverified.
