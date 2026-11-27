# internal/customization/

Customization (ADR 0026, design file `docs/design/09-customization.md`): how a project plugs in without a condition in the core. Configuration resolves core, packs, instance, tenant, project, task.

| Package | Owns |
|---|---|
| `config` | instance and tenant YAML, roles, schedules, file secrets, schema validation |
| `stack` | stack packs (`stacks/` and a project's own packs) and their tools |
| `onboard` | discovery of a repository, `tenant doctor`, the onboarding report |

## Rules

- Imports only customization; `internal/archtest` enforces it. Every plane but Data may read it; it reads no book, harness or adapter.
- `config` and `stack` are worker roots: nothing they reach may import the book or a Postgres client.
- A missing feature is a pack or a schema field (`schemas/`, skill `change-schema`), never a tenant name in code.
