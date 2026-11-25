---
status: accepted
date: 2026-11-25
---

# Several tenants share one instance, and an agent configures them through files

## Context and Problem Statement

The owner wants one locally running factory to build several projects at once, and wants an agent connected to it to create and configure a tenant completely: stack, skills, Lead's instructions, gates, review mode. The code already loads every `tenants/<name>/` directory and the ticker advances each tenant, but several fields have no effect: `capacity.tasks`, `resources` and `budget.weight` are in the schema and nothing reads them. Jobs are claimed oldest first across all tenants, so a long queue of one tenant starves the others. The worker shares its HOME and work volumes and mounts every mirror, and `claude-sub` allows one session, so parallel projects would still run one after another. There is no command that validates a whole configuration without starting the service.

## Considered Options

- Keep one tenant per instance; run several instances for several projects
- Several tenants per instance with per-tenant caps, fair claims and cleaned worker state, and configuration through files, a validator and a skill
- Several tenants per instance, configured by write tools in the MCP door

## Decision Outcome

Chosen option: "Several tenants per instance, configured through files, a validator and a skill", because the book, the router and the ticker already separate tenants, the missing pieces are small and testable without models, and a PR is the review and the audit trail the project already trusts. The trust boundary stays the instance (design section 17), so tenants of one instance share the machine, the model credentials and the egress filter.

- `nightshift config validate` checks instance, providers, tenants and, with `--repo-dir`, a project's `.nightshift/` files, and the references schemas cannot see. No secrets, no network.
- The `configure-tenant` skill edits the files, validates and opens a PR. It never edits secrets, `deploy/` or egress, and lowering `review.mode` stays the owner's decision.
- Enforce `capacity.tasks`, claim fairly by `budget.weight`, give each tenant its own conductor loop, restrict a job to its tenant's mirror and clean HOME and work between tenants, and read code-host credentials per tenant.
- Parallel builds need a provider with `concurrency` above 1. Choosing and paying for one is the owner's decision; the factory does not add a pool.

### Consequences

- Good: several projects run in parallel on one laptop with one `docker compose up`; the owner's agent can onboard a project without a human editing YAML.
- Good: every change to a tenant is a reviewed PR.
- Bad: tenants share egress hosts, the Postgres role and model credentials; a hostile tenant is not contained. Mitigation: one trust domain per instance, stated in the design.
- Bad: a new tenant needs a restart of `nightshift` until a reload exists. Mitigation: documented; reload waits for a second user.

## Pros and Cons of the Options

### One tenant per instance

- Good: strongest separation, no new code.
- Bad: one Postgres, one egress, one chat app and one stack per project; the second project costs a whole setup, against "one `docker compose up`".

### Write tools in the MCP door

- Good: no checkout needed.
- Bad: bypasses the PR review and merge gates; a prompt-injected agent could change a tenant's review mode. Revisit later, through PRs only.
