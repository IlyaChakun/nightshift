# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-24

## Last done

- Phase 0 is done. PRs #1–#10 are merged into `main`. Contracts: config and role result
  schemas, harness + `fake`, jobs API, integration interfaces, Slack manifest, egress
  allowlists, four ADRs, runbook skeleton, English `docs/design.md`.
- Phase 1 items 1–8: book schema, migrations, roles and sqlc queries; `internal/flow` state
  machine (T2 properties); instance and tenant loading, file secrets; River and the masked outbox;
  chat adapter over Socket Mode, its twin and the people policy; `nightshift serve`
  answers the owner in the thread (T4-lite on the twin), health endpoint, OTLP traces;
  `deploy/docker-compose.yml` with migrate step, hourly dumps, nightly restic, weekly restore test.
- `make vuln` fails on any known advisory (#13); Go style standard and stricter `.golangci.yml`.
  Go code review against it: a dead chat connection now stops `serve` (#34), `t.Context()` in tests (#35).
- Secrets review: `book_backup` role, `os.Getenv` banned, URL passwords masked, one file per secret.
- Design: onboarding (section 17); engineering disciplines (section 9).

## Next

- Phase 2 item 1: draft PR #45 (migration 00003, jobs queries, T3 green locally); after #44 merge
  main into it, tick item 1, merge on green CI.
- Then item 2. Ask-first PRs of phases 2–3 self-merge on green CI (owner), with the label.

## Blockers

- `mise.lock` is missing: run `mise lock --platform linux-x64,linux-arm64,macos-arm64` where
  GitHub downloads work and commit it; until then CI resolves tool checksums on every run.
- The Slack check needs the app from `deploy/slack-manifest.yaml` and its tokens (owner, local).

## Questions for the owner

- Delete branch `test/ci-red` (its fake key is in `.gitleaksignore` until then) and merged branches.
- #33 (shared schema registration) adds Go code under `schemas/`: approve or move it to `internal/config`.
- Unused contracts (`integration.Tracker`, `DocsTarget`, `ExternalChecks`, `policy.CanDecideOnGitHub`)
  and the AGENTS.md layout, which omits `internal/config`, `internal/integration`, `internal/testutil`.
- squawk and hadolint (design section 5) into `mise.toml` and `make verify`: now in a `needs-owner-review` PR (recommended) or in phase 3?
