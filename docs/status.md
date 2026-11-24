# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-24

## Last done

- Phase 0 is done. PRs #1–#10 are merged into `main`. Contracts: config and role result
  schemas, harness + `fake`, jobs API, integration interfaces, Slack manifest, egress
  allowlists, four ADRs, runbook skeleton, English `docs/design.md`.
- Training PR #11 turned CI red for a lint error, a reachable vulnerable dependency and a
  fake secret, then was closed without merging.

## Next

- Phase 1 (book and core): take the first open item once `docs/plan.md` lists phase 1.

## Blockers

- `mise.lock` is not generated yet: run `mise lock --platform linux-x64,linux-arm64,macos-arm64`
  where GitHub release downloads are allowed, and commit it. Until then CI resolves tool
  checksums on every run, and this once failed on a golangci-lint provenance check.

## Questions for the owner

- A vulnerable module that is imported but whose vulnerable code is never called passes
  `make vuln`: govulncheck ignores unreached code, and osv-scanner found nothing for
  `jwt-go v3.2.0+incompatible`. Accept this, or fail on any known advisory in `go.mod`?
- Delete branch `test/ci-red` (the session's proxy refused the delete). Its fake key is in
  `.gitleaksignore` meanwhile, because CI checks out every branch and gitleaks scans them all.
- Merged branches of the phase 0 stack can be deleted too.
