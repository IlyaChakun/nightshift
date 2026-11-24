# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-24

## Last done

- Plan items 5a and 5b: config validator (`internal/config`) and schemas for tenant, project,
  blast-radius, instance, providers and role; synthetic tenants and instances with negative tests.
- Plan item 6: role result schemas (lead-reply, spec, builder-result, verdict, finding,
  rubric-result, eval-result) with examples in `testdata/results/`.
- Plan item 7: `Harness` interface, schema-validating wrapper, `fake` adapter replaying
  `testdata/scenarios/happy-path.yaml`, golden helper (`-update`).
- Plan item 8: jobs API contract (`internal/jobsapi`), integration interfaces (`internal/integration`).
- Plan item 9: Slack manifest, egress allowlists for the worker and the CI runner.

## Next

- Plan item 10: first ADRs and runbook.
- Plan item 4: CI workflow that runs `make verify`.
- Plan item 2: English `docs/design.md`.

## Blockers

- `mise.lock` is not generated yet: run `mise lock --platform linux-x64,linux-arm64,macos-arm64`
  where GitHub release downloads are allowed, and commit it.

## Questions for the owner

- Approve phase 0 dependencies: jsonschema v6, go.yaml.in/yaml v3, rapid, actions/checkout,
  jdx/mise-action (asked in chat).
- Approve the egress allowlists in `deploy/egress-acl.yaml` and `deploy/egress-ci-acl.yaml`.
