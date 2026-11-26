# Status

> Phases 0–4 are built; phase 5 (Lead) meets its Done criterion on fakes and local Postgres, nothing of it ran live; phase 6 has no code. Next: task intake N1–N4, then plan S2 and the owner's test of the factory.

Rewrite at the end of every session. Limit: 40 lines and 600 words (skill `write-docs`). No history. Updated: 2026-11-26.

## Last done

- Phases 0–4 are built (#1–#190). Live: `make drill` passed on the owner's Mac (22 of 22, 2026-11-25: restore, restart, no token, no egress); one chat idea reached `released` on the `nightshift` tenant (#148); `claude-sub` and `codex-api` Builder jobs produced patches that passed `verify`.
- Phase 5 (2026-11-26, #276–#287): worker image has `standards` (decision 8); memory prune drops entries that the repo now states; tracker projection (twin only); `nightshift search` and `/ns search`; Lead answers a duplicate with a link; hidden scenarios are stored for Lead and Verifier; PRD and ADR drafts pass doc-lint before the owner sees the spec. All of it ran on the `fake` harness and local Postgres 16 tests, none with a live Lead, Slack, tracker or GitHub.
- Several tenants (ADR 0015): plan C1, C2, M1, M2, M4, M5 are done, M3 is half done (no checkout outlives its job). Plan R1 (a timeout per conductor step, #265) and R2 (repeat-safe side effects, #263, #264) are done.
- Research: ADR 0018 says no to Temporal; `docs/research/` and `docs/prior-art.md` hold it (#262, #266). It found one real gap: no stall, token, dollar or loop limit for any harness (plan S1 and S2). S1 (ADR 0019, #267): a supervisor stops a stalled, looping or overspending run for any harness, limits per role and tenant; the live check of the claude and codex streams (Mac) is open. S2: `nightshift status --task <id>` (#283) and the Grafana "Task timeline" dashboard as `grafana_ro`; the dashboard is not yet seen on the Mac. Grafana in `otel-lgtm` lets anonymous visitors in as Admin by default (127.0.0.1 only); the design's admin password is not wired yet.
- Research 2026-11-26: safety nets of other factories (`docs/research/safety-nets.md`); plan N5, N6, L5 added, none built. Read through page summaries only.
- Setup Z1–Z5 and the door items I1–I5 are merged. Phase 6 has no code.

## Next

- Task intake N1–N4 (`docs/research/task-intake.md`): the understood line, the clarity check, owner quotes per criterion, vague cases in the Lead evals.
- Open and not needed for Done: voice, #support reproduction, M3 part 2 (a HOME per tenant), Q1, Q2. `HiddenScenarios` has no reader yet: the Verifier uses it in P5.
- S1 and S2 wait for live checks on the Mac (offline since 2026-11-26 09:00 UTC). Phase 6 P1 and P2 may run in a parallel thread (decision 16).
- Live check wanted before the owner's own project: one idea in Slack through a real Lead to a spec card, a duplicate, and the tracker.

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: the pinned `hadolint` 2.12.0 (x86_64) segfaults; `gitleaks dir` also scans the ignored `.env`.
- PR #253 (Stop hook) is a draft: it needs one interactive Claude Code session on the Mac to confirm the hook loads from project settings.
- PR #233 conflicts with `main` and `docs/prior-art.md` supersedes it: close it.
- Not verified live: two tenants at once, the tenants bind mount, the one-click GitHub App creation, a worker death sweeping leases, side effects repeating on real GitHub (R2), the remote MCP door (decision 7).

## Questions for the owner

All of them are in `docs/decisions.md`; this file keeps no list of its own.
