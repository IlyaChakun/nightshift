# docs/

Write every document with `.agents/skills/write-docs`: summary first, one fact in one place, no filler.

## Which document answers what

| File | Question |
|---|---|
| `design.md` | what the factory is and why (the only living design; `design.html` is its view) |
| `adr/NNNN-*.md` | why one decision was taken (MADR 4.0, at least two options) |
| `plan.md` | what is open, by phase (limit 150 lines, 3200 words) |
| `status.md` | where we are now (limit 40 lines, 600 words; rewrite, never append) |
| `decisions.md` | what the owner still has to decide |
| `runbook.md`, `setup.md`, `secrets.md` | how a person runs, installs and configures it |
| `prior-art.md`, `research/` | what other systems do |

## Rules

- Write as current: no "v2", "replaces", "was/now".
- A design change updates `design.html` in the same PR, then `scripts/design-sync.sh --update` after reading the changed sections. Use `.agents/skills/change-design`.
- An architecture change needs an ADR in the same PR.
- Check that mermaid diagrams render and YAML examples parse. `make docs` runs markdownlint.
- Facts about the outside world carry a link and a date, or are marked unverified.
