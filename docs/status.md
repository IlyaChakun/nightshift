# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-24

## Last done

- Phase 0 plan items 1–10 are implemented as stacked PRs #2–#10 (#1 merged).
  Contracts: config and role result schemas, harness + `fake`, jobs API, integration
  interfaces, Slack manifest, egress allowlists, four ADRs, runbook skeleton.

## Next

- Merge the stack in order once CI is green and the owner's batch is approved.
- Plan item 11: training PR (secret, vulnerable dependency, lint error) must fail CI three ways.
- Then phase 0 is done; phase 1 starts with the book schema and migrations.

## Blockers

- `mise.lock` is not generated yet: run `mise lock --platform linux-x64,linux-arm64,macos-arm64`
  where GitHub release downloads are allowed, and commit it.

## Questions for the owner

- Approve phase 0 dependencies: jsonschema v6, go.yaml.in/yaml v3, rapid, actions/checkout,
  jdx/mise-action (asked in chat).
- Approve the egress allowlists in `deploy/egress-acl.yaml` and `deploy/egress-ci-acl.yaml`.
