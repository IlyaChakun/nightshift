# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-24

## Last done

- Phases 0 and 1 are done in code (PRs #1–#43); the Slack live check is the owner's local test.
- Phase 2 items 1–9 are merged or in review: jobs tables and queries (#45), jobs API server and
  client (#48), gitops mirrors and patch push (#49), `nightshift work` runner (#51), claude
  adapter (#52), provider router and task budget (#53), worker image, Smokescreen egress and
  the `jobs` network (#54), hidden scenarios never reach a Builder (#55), glue that runs claude
  jobs (#57), obs profile with `TRACEPARENT` to the agent.
- Test quality: outbox redelivery fix (#46), `-shuffle` with a seed and goleak in chat adapters (#50).
- CI caches for Go build, modules and golangci-lint (#56).
- The Slack app was created from `deploy/slack-manifest.yaml` (app id A0CLMBZ5D50).

## Next

- Phase 2 item 10 (owner): a real `claude` job shows in Grafana as one trace. Needs an API key
  in `deploy/secrets/harness/ANTHROPIC_API_KEY`, `CLAUDE_CODE_VERSION` set at the first build,
  and the obs steps in `docs/runbook.md`.
- Phase 3: Builder, checks, gates (design section 22), starting with `nightshift check`.
- Queued: squawk and hadolint in `make verify` (`mise.toml` tool names must be checked on a machine with mise).

## Blockers

- `mise.lock` is missing: run `mise lock --platform linux-x64,linux-arm64,macos-arm64` where
  GitHub downloads work and commit it.
- Slack: install the app to the workspace (bot token) and create the app-level token with
  `connections:write`, then put both in `deploy/secrets/` (runbook). The config token was shown in chat: revoke it.
- Compose and the images were never built here (no Docker). First `docker compose build` may
  need small fixes: Smokescreen flags, Alpine packages.

## Questions for the owner

- Delete branch `test/ci-red` (its fake key is in `.gitleaksignore` until then) and merged branches.
- #33 (shared schema registration) adds Go code under `schemas/`: approve or move it to `internal/config`.
- Unused contracts (`integration.Tracker`, `DocsTarget`, `ExternalChecks`, `policy.CanDecideOnGitHub`)
  and the AGENTS.md layout, which omits `internal/config`, `internal/integration`, `internal/testutil`
  (draft PR #39 aligns the layout lists).
