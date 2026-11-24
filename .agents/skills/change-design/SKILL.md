---
name: change-design
description: Change docs/design.md, its interactive page docs/design.html or an ADR. Use for any design, architecture or flow change, and when make verify fails with a design-sync or design-term error.
---

# Change the design

`docs/design.md` is the only living design. `docs/design.html` is its view. `AGENTS.md` and the owner's latest message win over the design; name any conflict and propose a fix to the stale text.

1. **Write it as current.** No "v2", "replaces", "was/now". English only.
2. **ADR when architecture changes.** Copy `templates/adr.md` to `docs/adr/NNNN-<topic>.md` (MADR 4.0, at least two considered options, `status`, `date`). The design and the ADR land in one PR.
3. **Update the page.** Every top-level section is `covered <hash>` or `skipped` in the `design-sync` manifest at the top of `docs/design.html`. If you changed a covered section, read the page's matching part and update it; every `data-term`, `term:` and `st:` value must exist in the design text.
4. **Record hashes last.** Run `scripts/design-sync.sh --update` only after the page reflects the change. Never refresh hashes without reading the changed sections; this is how CI went red on main for section 13.
5. **Diagrams render.** Mermaid blocks must parse; check before pushing. YAML examples must parse.
6. **Run** `make docs repo-check`. markdownlint runs on every `*.md`, including these skills.

A design change that touches gates, review policy, secrets or egress needs the owner's approval.
