# internal/assurance/

Governance and assurance plane (ADR 0026, design file `docs/design/05-assurance.md`): what makes a change trustworthy. A gate is code, never an agent's opinion.

| Package | Owns |
|---|---|
| `check` | tool reports to findings, the baseline, doc-lint (`nightshift check`) |
| `policy` | blast radius, review policy, constitution |
| `standards` | the L0 checklist from the standards packs |
| `rubric` | L4 rubric cards and the judge's answers |
| `deps` | the checks before a new dependency |

## Rules

- May import assurance, customization and the `harness` port only; `internal/testing/archtest` enforces it. No book, no adapters: callers pass the data in.
- It judges and returns results; pushing, merging and messages stay with control.
- `policy` is a critical path in `.nightshift/blast-radius.yaml`: a person reviews every change.
- A new tool, rubric or rule is data (a stack pack, a card under `standards/`) before it is code.
