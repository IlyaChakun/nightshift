# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-24

## Last done

- Phase 0 is done. PRs #1–#10 are merged into `main`. Contracts: config and role result
  schemas, harness + `fake`, jobs API, integration interfaces, Slack manifest, egress
  allowlists, four ADRs, runbook skeleton, English `docs/design.md`.
- Phase 1 items 1–7: book schema, migrations, roles and sqlc queries; `internal/flow` state
  machine (T2 properties); instance and tenant loading, file secrets; River and the masked outbox;
  chat adapter over Socket Mode, its twin and the people policy; `nightshift serve`
  answers the owner in the thread (T4-lite on the twin), health endpoint, OTLP traces.
- `make vuln` now fails on any known advisory in a dependency (#13), x/text, x/crypto and gorilla/websocket bumped.
- Training PR #11 turned CI red for a lint error, a reachable vulnerable dependency and a
  fake secret, then was closed without merging.

## Next

- Phase 1 item 8: compose for postgres and nightshift with backups. `serve` reads the secrets
  `pg_owner` and `pg_nightshift` (Postgres URLs), `chat_bot_token` and `chat_app_token`.

## Blockers

- `mise.lock` is not generated yet: run `mise lock --platform linux-x64,linux-arm64,macos-arm64`
  where GitHub release downloads are allowed, and commit it. Until then CI resolves tool
  checksums on every run, and this once failed on a golangci-lint provenance check.

- Item 9 needs a Slack workspace with the app from `deploy/slack-manifest.yaml` installed and
  its tokens in `deploy/secrets/` on the home machine (owner action).

## Questions for the owner

- Delete branch `test/ci-red` (the session's proxy refused the delete). Its fake key is in
  `.gitleaksignore` meanwhile, because CI checks out every branch and gitleaks scans them all.
- Merged branches of the phase 0 stack can be deleted too.
