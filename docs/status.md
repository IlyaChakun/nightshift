# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-24

## Last done

- Plan items 5a and 5b: config validator (`internal/config`) and schemas for tenant, project,
  blast-radius, instance, providers and role; synthetic tenants and instances with negative tests.

## Next

- Plan item 6: role result schemas.
- Plan item 4: CI workflow that runs `make verify`.
- Plan item 2: English `docs/design.md`.

## Blockers

- `mise.lock` is not generated yet: run `mise lock --platform linux-x64,linux-arm64,macos-arm64`
  where GitHub release downloads are allowed, and commit it.

## Questions for the owner

- Approve Go dependencies `github.com/santhosh-tekuri/jsonschema/v6` (Apache-2.0)
  and `go.yaml.in/yaml/v3` (MIT and Apache-2.0).
