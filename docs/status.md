# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-25

## Last done

- Phases 0 and 1 are done in code (#1–#43); their live Done checks (restore, restart, no token, no egress) are not verified yet, plan A2. Phase 2 items 1–10 and the codex adapter are done. Phase 3: items 1–12, 14–18 (the scenario runner replays every `testdata/scenarios/*.yaml`) and I1–I5 (doors) are done. Item 13 is done: on the `nightshift` tenant a chat idea reached `released` (#148).
- First real run on the owner's Mac (#104–#108): stack healthy, egress allowlist and secret isolation checked live, `claude-sub` and `codex-api` Builder jobs produced patches that passed `verify`; a task has one trace; Lead answers in Slack, with a `basis` (AGENTS.md "Evidence").
- Design adds research before building (section 6, ADR 0007; phases 4 and 5) and any project language through mise, `setup` and project packs (sections 5, 17; ADR 0009, 0010), which opens phase 4. Handoff, memory page and doors: ADR 0005, 0006.
- Phase 4 any-language path: `mise install` and `setup` through the `toolchain` service (ADR 0010), Debian worker, packs js/java-maven/java-gradle, skills web-dev/mobile-dev, `deps check`. PRs now get an independent review before merge (AGENTS.md).
- L4 rubrics (phase 4): YAML cards for PRD and ADR, `internal/rubric` with a `Judge` interface and a scripted fake, `check.RubricFindings`; a planted weak PRD gets L4 findings, a good one none. `rubric.HarnessJudge` (any harness) and `nightshift check --standards --judge-auth` run them; the worker image still lacks `standards` (Dockerfile is owner-gated), so the runner does not call them yet.
- Polygon runs locally (ADR 0011): `nightshift eval run` makes a throwaway git repo per task in `evals/tasks/`, hidden tests grade it, no GitHub; one synthetic task so far. Memory page storage: `nightshift memory show|remember`, revision CAS, 40 entries / 4 KB; `memory[]`, prune, prompt context open.
- A stable instance builds the factory itself (runbook, "Stable instance"): `low` risk merges
  itself. Gaps: a return job starts from `main` without the last patch, a failed check's log or
  (after the first return) the review text; task text is cut at 120 characters.

## Next

Phase 5, Lead, is in progress. Full autonomy since 2026-11-25 (ADR 0013): merge on green after the independent review; ask the owner only about secrets, egress hosts, money, irreversible steps, weakened protections. Next, in order: (1) morning digest: 09:00 task and `/ns status` list merged PRs and decisions with reason and revert link, reuse the L4 digest of #194; (2) memory page: `/ns` commands, `memory[]` in Lead's result, nightly prune, page in Lead's and Builder's context; (3) `needs_research` back to Lead, hidden scenarios, tracker projection, search, #support, voice. Open PR: #202 (fork CI and mirror; held until its two review defects are fixed and re-reviewed).

- Audit follow-up (plan A1–A5): `docs/decisions.md` is the one question list; A3 done (a test keeps the design's compose example and subcommand list equal to the code); next `make drill` (A2), unverified-facts rule (A4), limits (A5).
- Setup S1–S5 merged; the one-click app creation is not yet confirmed live. The factory's PRs are named by the approved spec; a task without a spec keeps its text.
- `nightshift skills sync` is built (phase 4); `skills/vendor.yaml` stays empty until the owner approves a starter list, so the worker still skips external skills with a warning.
- Codex cost needs `price` on its provider (docs/setup.md); its CLI tries `chatgpt.com` and GitHub, blocked by egress, run unaffected.

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: the pinned `hadolint` 2.12.0 (x86_64) segfaults; `gitleaks dir`
  also scans the ignored `.env`.

## Questions for the owner

All of them are in `docs/decisions.md`; this file keeps no list of its own.
