# Engineering capabilities

> Part of the [Nightshift design](../design.md). How the factory gains an engineering capability, such as a new language, a frontend or a deploy target, by composing parts that other planes already own. This is a map, not a runtime: it has no package folder, no plugin framework and no role of its own.

## 29. Capabilities as compositions

**A capability is a set of parts, each owned by its plane.** The factory can do a kind of work when every part exists for it.

| Part | What it answers | Where it lives today |
|---|---|---|
| Toolchain and runtime | Which tools build and run the code | Stack packs in `stacks/` and the worker's toolchains (section 5) |
| Skills and standards | How agents do the work and what good looks like | `skills/`, `standards/` and the project's own; disciplines (section 9) |
| Tools and permissions | What an agent may call and reach | Harness tool lists (section 12), the egress allowlist (section 16) |
| Verification | How an independent check proves the result | `verify`, test levels and gates (sections 8, 20) |
| Deployment targets | Where it runs, how it is released and rolled back | `capabilities` in `.nightshift/project.yaml`, deploy targets (section 17) |
| Observability | What is watched after a release | Smoke and health checks (section 27) |

The `capabilities` key of `project.yaml` holds only the delivery part (merge train, preview, release, rollback, smoke). A discipline of section 9 is the skills, standards and verification part. A capability in this section is the whole row set.

**The effective capability is the narrowest grant.** What the factory ships is limited by the instance (providers, egress), the tenant (data class, budgets, review policy), the project and the task's blast radius, along the customization axis of section 17. A layer cannot grant what a layer above it forbids: a project cannot use an egress host the instance does not allow.

**Rules.**

- No agent per discipline. Lead, Builder and Verifier do the work with the capability's skills (section 6).
- Start from the existing packs and a real tenant that needs the capability. An interface grows only when a second implementation needs it.
- A capability counts as available only when its verification ran on a real project. A pack on disk is not a capability.

**Examples of direction.** These are candidates, not claims that they exist; [PR #352](https://github.com/IlyaChakun/nightshift/pull/352) holds the research.

| Capability | Today | Candidate tools | Independent check |
|---|---|---|---|
| Backend | Stack packs `go`, `python`, `java-maven`, `java-gradle` | | Unit, integration and contract tests |
| Frontend | Packs `ts` and `js`, Playwright acceptance by Verifier | Browser tooling | Playwright, accessibility and visual checks |
| Mobile | none | Android and iOS toolchains, simulators | Device tests; signing and release on a person's button |
| Infrastructure | The `compose-local` deploy target | Terraform, `compose-ssh`, Kubernetes | Plan, policy, smoke, health, rollback |
| Data engineering | none | SQL, dbt, data contracts | Compatibility, data quality, lineage evidence |
| SRE | The SRE discipline (section 9) | OpenTelemetry, Prometheus | SLO, error budget, a recovery that actually ran |
