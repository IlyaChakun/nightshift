# Status

> Phases 0–4 are built; phase 5 (Lead) meets its Done criterion on fakes and local Postgres, nothing of it ran live; phase 6 has P1, P2 and P4 on fakes and stubs, none against a real Docker engine. Next: the owner's test of the factory.

Rewrite only when something true across phases changes. Limit: 40 lines and 600 words (skill `write-docs`). No history. Updated: 2026-11-26.

Each phase's last done, next step and open work are in the "Status" block at the top of its file under `docs/plan/` (index: `docs/plan.md`). Update yours there, not here: this file is shared by every thread.

## Across phases

- Live: `make drill` passed on the owner's Mac (22 of 22, 2026-11-25: restore, restart, no token, no egress); one chat idea reached `released` on the `nightshift` tenant (#148); `claude-sub` and `codex-api` Builder jobs produced patches that passed `verify`.
- Live checks waiting for the Mac: S1, S2, S3 (`docs/plan/phase-5.md`); phase 6 deployer; one idea through a real Lead.
- Merge the latest `main` into a branch and wait for green before merging it: two PRs both added migration 00015 (fixed in #297).

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: the pinned `hadolint` (x86_64) segfaults; `gitleaks dir` scans the ignored `.env`.
- PR #253 (Stop hook) is a draft: it needs one interactive Claude Code session on the Mac to confirm the hook loads from project settings.
- PR #233 conflicts with `main` and `docs/prior-art.md` supersedes it: close it.
- Not verified live: two tenants at once, the tenants bind mount, the one-click GitHub App creation, a worker death sweeping leases, side effects repeating on real GitHub (R2), the remote MCP door (decision 7).

## Questions for the owner

All of them are in `docs/decisions.md`; this file keeps no list of its own.
