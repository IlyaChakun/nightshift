# Stream — several tenants and agent-made configuration

Owned by the session working on this stream; another session asks it and does not edit this file. Rules and limits: `docs/plan.md`.

Done when: one instance builds several projects at once; an agent configures a tenant through files and a PR (ADR 0015).

## Status

- Last done: C1, C2, M1, M2, M4, M5; M3 is half done (no checkout outlives its job, a HOME per tenant).
- Next: M3 (mise data the agent cannot write, `/tmp` and the rest of the work root, replicas sharing `/work`, own mirror only).
- Not verified live: two tenants at once, the tenants bind mount, the one-click GitHub App creation.

## Items

One instance builds several projects at once; an agent configures a tenant through files and a PR. One item per PR, in order. The phase 5 thread owns `roles/` and `lead.go`; none of these edit them.

- [x] C1. `feat(cli): nightshift config validate` — instance, providers, tenants and (`--repo-dir`) `.nightshift/` files against `schemas/`, plus references (providers, skills, packs, repos); no secrets or network; T1, T3
- [x] C2. `docs(skills): own skill configure-tenant` — stack, skills, Lead instructions, gates, review mode via files and a PR, with C1 and `tenant doctor`; never secrets, `deploy/` or egress; runbook updated
- [x] M1. `feat(dispatch): capacity.tasks per tenant` — a queued task over the cap stays `queued`; a freed slot goes to the first queued task; T1, T3
- [x] M2. `feat(book): claim fairly across tenants` — oldest job of the tenant with the fewest running jobs per `budget.weight`, a provider's `concurrency` enforced at the claim under a lock, expired leases swept there (#250); one conductor loop per tenant, one tenant per repository; T2, T3.
- [ ] M3. `feat(runner): isolate worker state between tenants` — done: no checkout outlives its job, planted links not followed; T6. a HOME per tenant (`/home/agent/tenants/<tenant>`). Open: mise data the agent cannot write, `/tmp` and the rest of the work root, replicas sharing `/work`, own mirror only
- [x] M4. `feat(config): code-host credential per tenant` — `deploy/secrets/tenants/<name>/github_token`, instance fallback; `docs/secrets.md`; T1
- [x] M5. `docs(runbook): several projects on one laptop` — add a tenant (restart); raise `claude-sub` `concurrency` in `providers.yaml` within `weekly_share`, and a check that the sum of tenants' demand fits it. Later: reload without restart, a per-tenant provider share, MCP write tools through PRs only
