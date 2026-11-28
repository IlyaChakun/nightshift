# Capability composition and plugin architecture review

Use the existing project, role and stack configuration for the next onboarding need. The research identifies real composition mechanisms. It presents no measured duplication or compatibility failure that would pay for a plugin runtime.

Reviewed PR #352 against `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition on main |
| --- | --- |
| Nightshift already composes skills, stacks, role settings and adapters | Confirmed. Projects declare stack and skill entries, tenants configure providers and role overrides, and the runner assembles role instructions, project instructions and staged skills. `schemas/project.json:11`, `schemas/tenant.json:33`, `schemas/tenant.json:64`, `internal/runner/context.go:51`. |
| Project capabilities form a generic extension manifest | Not applicable. The closed schema lists concrete capabilities including merge and deploy. There is no generic manifest contract in this field. `schemas/project.json:32`. |
| Every declared prerequisite is mandatory | Partly. Unsynced external skills are skipped with a warning. A preset must specify which evidence is required and check that requirement independently. `internal/runner/context.go:75`, `internal/runner/context_test.go:93`. |
| Existing configuration is costly to repeat | Unverified. The research includes prospective presets, with no onboarding time, upgrade failure or duplicated maintenance measured for them. |

**Take when needed, phase 10.** Document one real workflow as a preset through the existing tenant configuration process. Record which role receives each skill, tool and input, what output it must produce and how required evidence is checked. Exercise it with synthetic configuration. Use the ordinary upgrade and rollback path to verify reproducibility.

**Later, phase 10.** If a second independently useful preset exposes the same version conflict or duplication, compare extending the pack against a small declarative manifest in an ADR. Record actual setup effort, effective scope, compatibility and version resolution first. Backend verification and advisory discovery are possible examples pending demand.

**Reject now.** A registry, dynamic native plugins, an executable WASM layer or a dedicated capability role each adds an execution or maintenance boundary. The Go package documentation also notes that a native plugin “cannot be closed.” Read 2026-11-26. [Go plugin source documentation](https://go.dev/src/plugin/plugin.go)

**Checked** Schemas and synthetic tenant fixtures. The role-context assembly and unsynced-skill tests passed in the runner package. **Inferred** Cost and pilot order. **Confidence** High for current configuration behavior. No two-preset deployment, scoped-tool experiment, upgrade or measured setup comparison was run.
