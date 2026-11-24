# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-24

## Last done

- Plan item 3: Go 1.27 module, `nightshift` command stubs, `make verify`
  (golangci-lint, tests, govulncheck, osv-scanner, gitleaks, markdownlint, repo-check).

## Next

- Plan item 5: config schemas and synthetic tenants `example-solo`, `example-team`.
- Plan item 2: English `docs/design.md`.
- Plan item 4: CI workflow that runs `make verify`.

## Blockers

- `mise.lock` is not generated yet: run `mise lock --platform linux-x64,linux-arm64,macos-arm64`
  where GitHub release downloads are allowed, and commit it.

## Questions for the owner

- None.
