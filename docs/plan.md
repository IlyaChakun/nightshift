# Plan

Phases and their "Done" criteria are defined in `docs/design.md` (section 22).
This file tracks the current phase only. Limit: 150 lines.
Rules: one item = one PR, under ~400 changed lines. Tick an item in the PR that completes it.
Add an item only if it follows from the design and the owner agreed.

## Current phase: 0 — skeleton and contracts

- [x] 1. `docs: add agent instructions, plan and status` — AGENTS.md, CLAUDE.md, README, plan, status
- [ ] 2. `docs: add design document` — English `docs/design.md`
- [x] 3. `chore: scaffold Go module and make verify` — go.mod, cmd/nightshift stubs, Makefile, mise, linters, line limits
- [x] 4. `ci: add verify workflow` — Actions pinned by SHA, `contents: read`, Dependabot
- [x] 5a. `feat(schemas): add tenant, project and blast-radius schemas` — validator, synthetic tenants
- [x] 5b. `feat(schemas): add instance, providers and role schemas` — synthetic instances, verifier role
- [x] 6. `feat(schemas): add role result schemas` — lead-reply, spec, builder-result, verdict, finding, rubric-result, eval-result
- [x] 7. `feat(harness): add harness interface and fake adapter` — schema check of results, golden helper
- [x] 8. `feat(jobsapi): define jobs API contract` — types, routes, integration adapter interfaces
- [x] 9. `feat(deploy): add Slack manifest and egress ACL`
- [x] 10. `docs: add first ADRs and runbook` — 4 ADRs (MADR 4.0), runbook, standards/templates/roles/skills dirs
- [ ] 11. Training PR with a secret, a vulnerable dependency and a lint error turns red in CI for each (closed, not merged)

## Phase 0 done when

- `make verify` is green on the skeleton.
- The training PR fails CI for each of its three defects.
- Every config example from the design, including `example-solo` and `example-team`, validates against its schema.

## Next phases

1 — book and core · 2 — runner, harnesses, isolation · 3 — Builder, checks, gates.
Phases 4–10 start only when the previous ones hurt without them (design section 22).
