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
- Phase 3 plan added; items 1–4 done: SARIF and baseline, `nightshift check`, role loader and Builder, the constitution (protected paths) in `gitops.ApplyPatch`.
- Outbox redelivery fix (#46); `-shuffle` and goleak (#50); CI caches (#56); Slack app created (A0CLMBZ5D50).

## Next

- Phase 2 item 10 (owner): a real `claude` job shows in Grafana as one trace. Needs an API key
  in `deploy/secrets/harness/ANTHROPIC_API_KEY`, `CLAUDE_CODE_VERSION` set at the first build,
  and the obs steps in `docs/runbook.md`.
- Phase 3 item 5: atomic state transitions (CAS in the book). Role prompts follow the chat style of PR 59.

## Blockers

- `mise.lock` is missing: run `mise lock --platform linux-x64,linux-arm64,macos-arm64` where
  GitHub downloads work and commit it.
- Revoke the Slack config token that was shown in chat.
- Compose and the images were never built here (no Docker). First `docker compose build` may
  need small fixes: Smokescreen flags, Alpine packages.

## Questions for the owner

- Delete branch `test/ci-red` (its fake key is in `.gitleaksignore` until then) and merged branches.
- #33 (shared schema registration) adds Go code under `schemas/`: approve or move it to `internal/config`.
- Unused contracts (`integration.Tracker`, `DocsTarget`, `ExternalChecks`, `policy.CanDecideOnGitHub`)
  and the AGENTS.md layout, which omits `internal/config`, `internal/integration`, `internal/testutil`
  (draft PR #39 aligns the layout lists).
