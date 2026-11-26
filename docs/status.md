# Status

> Phases 0–4 are built; phase 5 (Lead) meets its Done criterion on fakes and local Postgres, nothing of it ran live; phase 6 has P1, P2 and P4 on fakes and stubs (#278, #291, #292, #298), none against a real Docker engine. Next: plan S2, L5, N7, then the owner's test of the factory.

Rewrite at the end of every session. Limit: 40 lines and 600 words (skill `write-docs`). No history. Updated: 2026-11-26.

## Last done

- Phases 0–4 are built (#1–#190). Live: `make drill` passed on the owner's Mac (22 of 22, 2026-11-25: restore, restart, no token, no egress); one chat idea reached `released` on the `nightshift` tenant (#148); `claude-sub` and `codex-api` Builder jobs produced patches that passed `verify`.
- Phase 5 (2026-11-26, #276–#287): standards in the worker image, memory prune, tracker projection (twin only), `nightshift search`, duplicate links, hidden scenarios, PRD and ADR drafts through doc-lint. All on the `fake` harness and local Postgres, none with a live Lead, Slack, tracker or GitHub.
- Several tenants (ADR 0015): plan C1, C2, M1, M2, M4, M5 are done, M3 is half done (no checkout outlives its job). Plan R1 (a timeout per conductor step, #265) and R2 (repeat-safe side effects, #263, #264) are done.
- Research: ADR 0018 says no to Temporal; S1 (ADR 0019, a supervisor for stalled, looping or overspending runs), S2 (`nightshift status --task <id>`, Grafana "Task timeline") and S3 (ADR 0020, model and effort per role) are merged.
- Lead intake and safety nets N1–N6 are merged (#288–#304), including an optional independent reader (#304, off without a `reader` chain). Fakes only; its benefit is not measured.
- Setup Z1–Z5, doors I1–I5 merged. Digest capped, not live yet.
- Phase 6 (2026-11-26): the `Deployer` interface and twin (P1), deploy steps, preview gate, release and automatic rollback in the train (P2), and `compose-local` with its recipe lint, run by the `deployer` service under profile `deploy` (P4). Stubs and local git only.

## Next

- N7: a two-turn eval case for rule 8 (a correction updates spec and memory); L5: Lead run record (the S1 thread).
- Open: voice, #support reproduction, M3 part 2, Q1, Q2.
- Merge the latest `main` into a branch and wait for green before merging it: two PRs both added migration 00015 (fixed in #297).
- S1, S2 and S3 wait for live checks on the Mac (offline since 2026-11-26 09:00 UTC). Phase 6 open: P3 `script` and `external`, P5 live acceptance, P6 revert PR and preview teardown, the T4 deploy scenario.
- Mac, when back: `docker compose --profile deploy up -d --build` (the alpine `docker-cli` packages are unverified), a preview on 127.0.0.1, the token readable by the deployer, `deploy/init-secrets.sh` rerun.
- Live check wanted before the owner's own project: one idea in Slack through a real Lead to a spec card, a duplicate, and the tracker.

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: the pinned `hadolint` (x86_64) segfaults; `gitleaks dir` scans the ignored `.env`.
- PR #253 (Stop hook) is a draft: it needs one interactive Claude Code session on the Mac to confirm the hook loads from project settings.
- PR #233 conflicts with `main` and `docs/prior-art.md` supersedes it: close it.
- Not verified live: two tenants at once, the tenants bind mount, the one-click GitHub App creation, a worker death sweeping leases, side effects repeating on real GitHub (R2), the remote MCP door (decision 7).

## Questions for the owner

All of them are in `docs/decisions.md`; this file keeps no list of its own.
