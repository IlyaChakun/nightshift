# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-25

## Last done

- Phases 0 and 1 are done in code (#1–#43). Phase 2 items 1–10 and the codex adapter are done.
- Phase 3: items 1–7, 9, 11, 12, 14, 15 and most of 8 and 10 are merged.
- First real run on the owner's Mac (#104–#108): stack healthy, egress allowlist and secret
  isolation checked live, `claude-sub` and `codex-api` Builder jobs queued by SQL produced
  patches that passed `verify`; the conductor is wired into compose; a task has one trace.
  Slack accepts both tokens and the bot connects; no message has been exchanged yet.

- Design adds session handoff (section 12, plan 3.16) and the project memory page (section 13, phase 5); ADR 0005.

## Next

- `nightshift skills sync` (phase 4): until then the worker skips external skills with a warning.
- Item 13: a task reached `human_review` on `nightshift-sandbox`. With the GitHub App, approval,
  the train and release are next (`tenant doctor` and `init --open-pr` still want a token).
- Slack check: put the owner's member id and the channel ids into `tenant.yaml`.
- Codex reports no cost (`cost_usd` 0), so a pool budget cannot see its spend; its CLI also
  tries `chatgpt.com` and GitHub, which egress blocks without breaking the run.
- Phase 3 item 10: Linear twin, T4/T5 scenario runner; Lead chat loop.

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: the pinned `hadolint` 2.12.0 (x86_64) segfaults; `gitleaks dir`
  also scans the ignored `.env`.

## Questions for the owner

- Phase 2 item 10 is ticked on the recommended reading: Claude Code's own spans sit in separate
  traces, so model-call details come from the logs. Reopen it if one trace must hold them.
- Delete branch `test/ci-red` (its fake key is in `.gitleaksignore` until then) and merged branches.
- Pick a licence and approve the dev-process batch in the PR `docs: dev process batch`.
- The 37 MB binary removed in #104 is still in history: rewrite it (force-push) or leave it.
