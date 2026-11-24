---
name: change-schema
description: Add or change a JSON Schema in schemas/ (role results, tenant, project, finding, note, providers). Use when an agent result or a config file needs a new field or a new kind.
---

# Change a schema

`schemas/` is a gated path: the change needs the owner's approval stated in the PR and the label `needs-owner-review` (AGENTS.md, "Ask before you do it"). Open the PR as draft and batch the question with other owner items.

1. **Say why a schema and not code.** Agent results and config are data validated against `schemas/`; side effects stay in `nightshift` code. If the new field is not read by code, it probably does not belong.
2. **Edit the schema.** Embedded files are loaded by `schemas/schemas.go`; `schemas_test.go` compiles every `*.json`. Prefer additive, optional fields.
3. **Keep fixtures in step.** Each result schema has a valid sample in `testdata/results/<name>.json` and tests that validate it. Update the sample, and add one invalid case for the new rule.
4. **Update the readers.** Search for the schema name in `internal/` (loader, harness `validate.go`, `roles/*.yaml`). A role's output schema change also changes its prompt in `roles/<role>.md`.
5. **Docs.** If the design describes the field (section 6, 9, 12 or 17), update `docs/design.md` and `docs/design.html` in the same PR (skill `change-design`).

Never make a schema looser to pass a test; fix the producer.
