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
- CI: tests share one Postgres container per binary (`pgtest`); `verify` p50 went from 129 s to ~80 s.
- Design adds session handoff (section 12, plan 3.16) and the project memory page (section 13, phase 5); ADR 0005.

## Next

- Setup automation S1–S5 is merged; confirming the one-click app creation live is open.
- Item 13: on `nightshift-sandbox` (review off) a task went from `task add` to `released`: the
  App opened and merged it. Not run live: a person's approval; next: the `nightshift` tenant.
- `nightshift skills sync` (phase 4): until then the worker skips external skills with a warning.
- Codex reports no cost (`cost_usd` 0), so a pool budget cannot see its spend; its CLI also
  tries `chatgpt.com` and GitHub, which egress blocks without breaking the run.
- Phase 3 item 10: Linear twin, T4/T5 scenario runner.

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
