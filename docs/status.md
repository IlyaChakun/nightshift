# Status

> Phases 0–4 are built; phase 5 (Lead) meets its Done criterion on fakes and local Postgres, and its supervisor, timeline and model pins ran live; phase 6 has P1, P2 and P4 on fakes and stubs, and its first run on a real Docker engine found defects whose fixes await a re-run. What still owes a live check: `docs/plan/live-checks.md`. Next: the owner's test of the factory.

Rewrite only when something true across phases changes. Limit: 40 lines and 600 words (skill `write-docs`). No history. Updated: 2026-11-26.

Each phase's last done, next step and open work are in the "Status" block at the top of its `README.md` under `docs/plan/` (index: `docs/plan.md`). Update yours there, not here: this file is shared by every thread.

## Across phases

- Live: `make drill` passed on the owner's Mac (22 of 22, 2026-11-25: restore, restart, no token, no egress); one chat idea reached `released` on the `nightshift` tenant (#148); `claude-sub` and `codex-api` Builder jobs produced patches that passed `verify`.
- Merged work that still owes a check on a real stack is queued in `docs/plan/live-checks.md`, with what to run and the result of each check.

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: `gitleaks dir` scans the ignored `.env`. The pinned `hadolint` (x86_64 only) crashes there; `make docker-lint` now falls back to the official image (live check queued).
- PR #253 (Stop hook) is a draft: it needs one interactive Claude Code session on the Mac to confirm the hook loads from project settings.
- PR #233 conflicts with `main` and `docs/prior-art.md` supersedes it: close it.

## Questions for the owner

All of them are in `docs/decisions.md`; this file keeps no list of its own.
