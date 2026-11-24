# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-24

## Last done

- Plan item 5a: config validator (`internal/config`), tenant, project and blast-radius
  schemas, synthetic tenants `example-solo` and `example-team` with negative tests.

## Next

- Plan item 5b: instance, providers and role schemas.
- Plan item 4: CI workflow that runs `make verify`.
- Plan item 2: English `docs/design.md`.

## Blockers

- `mise.lock` is not generated yet: run `mise lock --platform linux-x64,linux-arm64,macos-arm64`
  where GitHub release downloads are allowed, and commit it.

## Questions for the owner

- Approve Go dependencies `github.com/santhosh-tekuri/jsonschema/v6` (Apache-2.0)
  and `go.yaml.in/yaml/v3` (MIT and Apache-2.0).
