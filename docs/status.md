# Status

> Phases 0–4 are built; phase 5 (Lead) is built except its long tail; phase 6 has no code. Next: the memory prune rule, hidden scenarios, tracker projection, `ns search`, #support, voice, then plan S1 and S2. Live checks are open (see Blockers).

Rewrite at the end of every session. Limit: 40 lines and 600 words (skill `write-docs`). No history. Updated: 2026-11-26.

## Last done

- Phases 0–4 are built (#1–#190). Live: `make drill` passed on the owner's Mac (22 of 22, 2026-11-25: restore, restart, no token, no egress); one chat idea reached `released` on the `nightshift` tenant (#148); `claude-sub` and `codex-api` Builder jobs produced patches that passed `verify`.
- Phase 5 is built except its long tail: Lead with the memory page and `memory[]`, `needs_research`, role overrides, `/ns status|memory|remember`, the morning digest (#221–#255). Three defects from the live stand are fixed in code (#246, #248, #251) and not re-checked live.
- Several tenants (ADR 0015): plan C1, C2, M1, M2, M4, M5 are done, M3 is half done (no checkout outlives its job). Plan R1 (a timeout per conductor step, #265) and R2 (repeat-safe side effects, #263, #264) are done.
- Research: ADR 0018 says no to Temporal; `docs/research/` and `docs/prior-art.md` hold it (#262, #266). It found one real gap: no stall, token, dollar or loop limit for any harness (plan S1 and S2). S1 (ADR 0019, #267): a supervisor stops a stalled, looping or overspending run for any harness, limits per role and tenant; the live check of the claude and codex streams (Mac) is open.
- Setup Z1–Z5 and the door items I1–I5 are merged. Phase 6 has no code.

## Next

- No thread is running phase 5: the build thread stopped at a usage limit on 2026-11-25 at 20:48 UTC and nothing woke it. Resume with: the prune rule for memory entries whose fact moved into the repo, hidden scenarios storage, tracker projection, `ns search`, #support, voice, M3 part 2 (a HOME per tenant), Q1, Q2.
- Then plan S1 (one supervisor for every harness; the harness change is the owner's call) and S2.
- Phase 6 waits for phase 5's Done criterion (`AGENTS.md`); P1 and P2 may start in a parallel thread (decision 16, taken).
- The worker image lacks `standards` (decision 8): `AGENTS.md` lets a session decide a `Dockerfile` change, so take it.

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: the pinned `hadolint` 2.12.0 (x86_64) segfaults; `gitleaks dir` also scans the ignored `.env`.
- PR #253 (Stop hook) is a draft: it needs one interactive Claude Code session on the Mac to confirm the hook loads from project settings.
- PR #233 conflicts with `main` and `docs/prior-art.md` supersedes it: close it.
- Not verified live: two tenants at once, the tenants bind mount, the one-click GitHub App creation, a worker death sweeping leases, side effects repeating on real GitHub (R2), the remote MCP door (decision 7).

## Questions for the owner

All of them are in `docs/decisions.md`; this file keeps no list of its own.
