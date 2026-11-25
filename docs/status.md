# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-25

## Last done

- Phases 0 and 1 are done in code (#1–#43); the Slack live check is still open.
- Phase 2 items 1–9 and the codex adapter are merged (#45–#57, 9a).
- Phase 3: items 1–7, 9, 11, 12, 14, 15 and most of 8 and 10 are merged: checks, roles, flow,
  merge train, conductor Builder, review and Verifier legs (`serve --repos`, off by default),
  onboarding, stack packs, memory, search tables.
- First real run on the owner's Mac: stack healthy, egress allowlist and secret isolation
  checked live, `claude-sub` and `codex-api` Builder jobs queued by SQL each produced a
  patch and passed `verify`. The fixes it needed are in the PR `fix: make the first real worker run work`.

## Next

- Item 13, conductor in compose: the `nightshift` image has no git, `/repos` is root-owned so
  uid 10001 cannot write mirrors, artifacts are readable by root only, and compose passes no
  `--repos` or `github_token`.
- Phase 2 item 10: no span per job, `build.go` jobs carry no `traceparent`, agent spans start
  their own traces (logs do carry the inbound trace id); Claude metrics were not seen.
- Codex: cost is not reported (`cost_usd` 0), so a pool budget cannot see its spend; the CLI
  also tries `chatgpt.com` and GitHub, which egress blocks without breaking the run.
- Phase 3 item 10: Linear twin, T4/T5 scenario runner; Lead chat loop.

## Blockers

- `mise.lock` (and `.mise/locks/`) is generated on the owner's Mac and waits for its own PR.
- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: the pinned `hadolint` 2.12.0 (x86_64) segfaults; `gitleaks dir`
  also scans the ignored `.env`.

## Questions for the owner

- Delete branch `test/ci-red` (its fake key is in `.gitleaksignore` until then) and merged branches.
- Pick a licence and approve the dev-process batch in the PR `docs: dev process batch`.
- The 37 MB binary removed in #104 is still in history: rewrite it (force-push) or leave it.
