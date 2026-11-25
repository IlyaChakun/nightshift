# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-25

## Last done

- Phases 0 and 1 are done in code (#1–#43). Phase 2 items 1–10 and the codex adapter are done.
- Phase 3: items 1–12, 14–18 (the scenario runner replays every `testdata/scenarios/*.yaml`) and I1–I5 (doors) are done. Item 13 is done: on the `nightshift` tenant a chat idea reached `released` (#148).
- First real run on the owner's Mac (#104–#108): stack healthy, egress allowlist and secret isolation checked live, `claude-sub` and `codex-api` Builder jobs produced patches that passed `verify`; a task has one trace; Lead answers in Slack.
- Design adds research before building (ADR 0007, phases 4 and 5), session handoff (plan 3.16),
  the project memory page (phase 5) and other doors besides Slack (I1–I5); ADR 0005, 0006.
- Audit of main after phase 3: tests and lint green; eight bugs fixed (serve hang, `AddIdea` repo check, Claude env keys, stale change requests, Slack long cards and more).

- A stable instance builds the factory itself (runbook, "Stable instance"; tag
  `factory-stable-2026-11-25.2`). On the owner's Mac the dev stack serves only the sandbox.

## Next

Next is phase 4; phases 4–10 follow in order (live Verifier is phase 6; its leg runs on `fake`).

- Setup S1–S5 merged; the one-click app creation is not yet confirmed live.
- `nightshift skills sync` (phase 4): until then the worker skips external skills with a warning.
- Codex cost needs `price` on its provider (docs/setup.md); its CLI tries `chatgpt.com` and GitHub, blocked by egress, run unaffected.

## Blockers

- Revoke the Slack config token that was shown in chat.
- Open from the audit: the inbox door drops an identical repeated card; `verifying` consumes the verdict before the move; Slack send errors are all retried (no `ErrPermanent`); the codex schema prune drops allowed nulls; `pgtest` external mode (`NIGHTSHIFT_TEST_PG`) collides when packages run in parallel (use `-p 1`).
- From dogfooding: a "request changes" review reaches Builder without its text; task text is cut
  at 120 characters; stack tools are skipped in the alpine worker.
- `make verify` on an arm64 Mac: the pinned `hadolint` 2.12.0 (x86_64) segfaults; `gitleaks dir`
  also scans the ignored `.env`.

## Questions for the owner

- Mutation tests: old plan said phase 4, design says "Later"; the plan follows the design.
- Phase 2 item 10 is ticked on the reading that model-call details come from logs.
- Pick a licence; approve the dev-process batch (PR #85). Delete `test/ci-red` and merged branches. The 37 MB binary removed in #104 is still in history: rewrite it or leave it.
