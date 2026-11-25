# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-25

## Last done

- Phases 0 and 1 are done in code (#1–#43). Phase 2 items 1–10 and the codex adapter are done. Phase 3: items 1–12, 14–18 (the scenario runner replays every `testdata/scenarios/*.yaml`) and I1–I5 (doors) are done. Item 13 is done: on the `nightshift` tenant a chat idea reached `released` (#148).
- First real run on the owner's Mac (#104–#108): stack healthy, egress allowlist and secret isolation checked live, `claude-sub` and `codex-api` Builder jobs produced patches that passed `verify`; a task has one trace; Lead answers in Slack, with a `basis` (AGENTS.md "Evidence").
- CI tests share one Postgres per binary (`verify` p50 ~80 s); docs-only PRs run only the document checks.
- Design adds research before building (section 6, ADR 0007; phases 4 and 5) and any project language through mise, `setup` and project packs (sections 5, 17; ADR 0009, 0010), which opens phase 4. Handoff, memory page and doors: ADR 0005, 0006.

- Phase 4 any-language path: `mise install` and `setup` through the `toolchain` service (ADR 0010), Debian worker, packs js/java-maven/java-gradle, skills web-dev/mobile-dev, `deps check`. PRs now get an independent review before merge (AGENTS.md).
- L4 rubrics (phase 4): YAML cards for PRD and ADR, `internal/rubric` with a `Judge` interface and a scripted fake, `check.RubricFindings`; a planted weak PRD gets L4 findings, a good one none. The `api` judge and the Verifier wiring come next.
- Audit of main after phase 3: eight bugs fixed (serve hang, `AddIdea` repo check, Claude env keys, stale change requests, Slack long cards and more).

## Next

Phase 3 plan items are done; next is phase 4. Owner request in parallel: limits and a night queue (ADR 0009, plan L1–L5; L1 done, next L2). Phases 4–10 follow in order; memory page
is phase 5, nightly learn phase 7, live Verifier phase 6 (its review leg runs on `fake` now).

- Setup S1–S5 merged; the one-click app creation is not yet confirmed live. The factory's PRs are named by the approved spec; a task without a spec keeps its text.
- `nightshift skills sync` (phase 4): until then the worker skips external skills with a warning.
- Codex cost needs `price` on its provider (docs/setup.md); its CLI tries `chatgpt.com` and GitHub, blocked by egress, run unaffected.

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: the pinned `hadolint` 2.12.0 (x86_64) segfaults; `gitleaks dir`
  also scans the ignored `.env`.

## Questions for the owner

- The toolchain service needs the install host list in `deploy/egress-install-acl.yaml` confirmed (mise, go.dev, nodejs.org, dl.google.com, github.com; Java and Rust hosts per instance). The mise apt repo in the Dockerfile is untested (no Docker in the sandbox); `docker compose build` on the owner's machine will show it. `base_url` and `context_tokens` in `schemas/providers.json` only when open models are built. Asked in one batch then.
- Mutation tests: old plan said phase 4, design says "Later"; the plan follows the design.
- Phase 2 item 10 is ticked on the recommended reading (model-call details come from logs).
- Repo is public (2026-11-25). Audit: no secrets in all history (gitleaks, all refs), no tenant names. Open: licence; main ruleset (require `ci-ok`, no force-push, App bypass); turn on secret scanning, push protection and Dependabot alerts; Settings > Actions: approval for fork PRs, read-only token; commit email in history is public.
- Approve the dev-process batch (PR #85). Delete `test/ci-red` and merged branches.
- The 37 MB binary removed in #104 is still in history (clean, no secrets): leave it, rewriting breaks PR refs.
