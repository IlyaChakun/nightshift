# internal/evolution/

Evolution loop (ADR 0026, design file `docs/design/06-evolution.md`): how the factory improves itself from its own findings.

| Package | Owns |
|---|---|
| `learn` | the deterministic half of the nightly lessons: clusters findings, picks what to fix |

## Rules

- May import evolution, the `harness` port, knowledge, assurance, data and customization; `internal/archtest` enforces it.
- It changes the factory only through pull requests that pass the same gates; nothing here merges, pushes or edits state of another plane.
- Root causes and fixes are the model's job behind `harness`; code here stays deterministic.
