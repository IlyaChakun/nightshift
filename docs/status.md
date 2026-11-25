# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-25

## Last done

- Phases 0 and 1 are done in code (#1–#43). Phase 2 items 1–10 and the codex adapter are done. Phase 3: items 1–12, 14–18 (the scenario runner replays every `testdata/scenarios/*.yaml`) and I1–I5 (doors) are done. Item 13 is done: on the `nightshift` tenant a chat idea reached `released` (#148).
- First real run on the owner's Mac (#104–#108): stack healthy, egress allowlist and secret isolation checked live, `claude-sub` and `codex-api` Builder jobs produced patches that passed `verify`; a task has one trace; Lead answers in Slack, with a `basis` (AGENTS.md "Evidence").
- Design adds research before building (section 6, ADR 0007; phases 4 and 5) and any project language through mise, `setup` and project packs (sections 5, 17; ADR 0009, 0010), which opens phase 4. Handoff, memory page and doors: ADR 0005, 0006.
- Phase 4 any-language path: `mise install` and `setup` through the `toolchain` service (ADR 0010), Debian worker, packs js/java-maven/java-gradle, skills web-dev/mobile-dev, `deps check`. PRs now get an independent review before merge (AGENTS.md).
- L4 rubrics (phase 4): YAML cards for PRD and ADR, `internal/rubric` with a `Judge` interface and a scripted fake, `check.RubricFindings`; a planted weak PRD gets L4 findings, a good one none. `rubric.HarnessJudge` (any harness) and `nightshift check --standards --judge-auth` run them; the worker image still lacks `standards` (Dockerfile is owner-gated), so the runner does not call them yet.
- Polygon runs locally (ADR 0011, owner 2026-11-25): `nightshift eval run` makes a throwaway git repo per task in `evals/tasks/`, hidden tests grade it, no GitHub; one synthetic task so far.
- A stable instance builds the factory itself (runbook, "Stable instance"): `low` risk merges
  itself. Gaps: a return job starts from `main` without the last patch, a failed check's log or
  (after the first return) the review text; task text is cut at 120 characters.

## Next

Phase 4 is closed (leftovers in docs/plan.md); next is phase 5, Lead. Owner request in parallel: limits and a night queue (ADR 0012, plan L1–L5; L1–L5 done). Phases 4–10 follow in order; memory page
is phase 5, nightly learn phase 7, live Verifier phase 6 (its review leg runs on `fake` now).

- Setup S1–S5 merged; the one-click app creation is not yet confirmed live. The factory's PRs are named by the approved spec; a task without a spec keeps its text.
- `nightshift skills sync` is built (phase 4); `skills/vendor.yaml` stays empty until the owner approves a starter list, so the worker still skips external skills with a warning.
- Codex cost needs `price` on its provider (docs/setup.md); its CLI tries `chatgpt.com` and GitHub, blocked by egress, run unaffected.

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: the pinned `hadolint` 2.12.0 (x86_64) segfaults; `gitleaks dir`
  also scans the ignored `.env`.

## Questions for the owner

- The toolchain service needs the install host list in `deploy/egress-install-acl.yaml` confirmed (mise, go.dev, nodejs.org, dl.google.com, github.com; Java and Rust hosts per instance). The mise apt repo in the Dockerfile is untested (no Docker in the sandbox); `docker compose build` on the owner's machine will show it. `base_url` and `context_tokens` in `schemas/providers.json` only when open models are built. Asked in one batch then.
- Mutation tests follow the design ("Later"); phase 2 item 10: model-call details come from logs. Approve the dev-process batch (PR #85).
- Repo is public (2026-11-25). Audit: no secrets in all history (gitleaks, all refs), no tenant names. On: `main` ruleset (PR, `ci-ok`), secret scanning with push protection, fork PR approval, private vulnerability reporting, CodeQL, Dependabot. Open: licence; commit email in history is public.
- Security follow-ups (review of #182), no outsider can command the factory: over 5000 reviews stall a task (fails closed); the git mirror also fetches `refs/pull/*`; unconfirmed: a fork's run on the same SHA may supersede CI in the Actions fallback.
- Delete `test/ci-red` and merged branches; the binary removed in #104 stays in history.
