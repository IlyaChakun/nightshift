# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-25

## Last done

- Phases 0 and 1 are done in code (#1–#43). Phase 2 items 1–10 and the codex adapter are done.
- Phase 3: items 1–7, 9, 11, 12, 14, 15 and most of 8 and 10 are merged.
- First real run on the owner's Mac (#104–#108): stack healthy, egress allowlist and secret
  isolation checked live, `claude-sub` and `codex-api` Builder jobs queued by SQL produced
  patches that passed `verify`; the conductor is wired into compose; a task has one trace.
  Slack accepts both tokens; no message has been exchanged yet.
- The design page shows phases and plan counts; CI tests share one Postgres per binary (`verify` p50 ~80 s).
- Design adds session handoff (section 12, plan 3.16), the project memory page (section 13, phase 5) and other doors besides Slack (section 15, plan I1–I5); ADR 0005, 0006.

## Next

Current phase: 3. Order (plan): item 18 Lead stage in Slack, 16 handoff, 17 codex cost, Linear twin
and T5 runner (10), then 13 live on the `nightshift` tenant. Phases 4–10 follow in order; memory page
is phase 5, nightly learn phase 7, live Verifier phase 6 (its review leg runs on `fake` now).

- Setup S1–S5 merged; the one-click app creation is not yet confirmed live. Item 13 passed on `nightshift-sandbox` (review off); a run with a person's approval is open.
- `nightshift skills sync` (phase 4): until then the worker skips external skills with a warning.
- Codex reports no cost (plan 3.17); its CLI tries `chatgpt.com` and GitHub, blocked by egress, run unaffected.
- Doors besides Slack (I1–I5) follow items 18, 16, 17; the MCP SDK dependency and the `inbox` schema value are approved in chat, I4 waits for the connector sign-in check (design section 25).

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: the pinned `hadolint` 2.12.0 (x86_64) segfaults; `gitleaks dir`
  also scans the ignored `.env`.

## Questions for the owner

- Mutation tests: old plan said phase 4, design says "Later"; the plan follows the design.
- Phase 2 item 10 is ticked on the recommended reading (model-call details come from logs).
- Pick a licence; approve the dev-process batch (PR #85). Delete `test/ci-red` and merged branches.
- The 37 MB binary removed in #104 is still in history: rewrite it (force-push) or leave it.
