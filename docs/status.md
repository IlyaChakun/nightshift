# Status

Rewrite this file at the end of every session. Limit: 40 lines. No history here.

Updated: 2026-11-25

## Last done

- Phases 0 and 1 are done in code (#1–#43); their live Done checks (restore, restart, no token, no egress) are not verified yet, plan A2. Phase 2 items 1–10 and the codex adapter are done. Phase 3: items 1–12, 14–18 (the scenario runner replays every `testdata/scenarios/*.yaml`) and I1–I5 (doors) are done. Item 13 is done: on the `nightshift` tenant a chat idea reached `released` (#148).
- First real run on the owner's Mac (#104–#108): stack healthy, egress allowlist and secret isolation checked live, `claude-sub` and `codex-api` Builder jobs produced patches that passed `verify`; a task has one trace; Lead answers in Slack, with a `basis` (AGENTS.md "Evidence").
- Design adds research before building (section 6, ADR 0007; phases 4 and 5) and any project language through mise, `setup` and project packs (sections 5, 17; ADR 0009, 0010), which opens phase 4. Handoff, memory page and doors: ADR 0005, 0006.
- Phase 4 any-language path: `mise install` and `setup` through the `toolchain` service (ADR 0010), Debian worker, packs js/java-maven/java-gradle, skills web-dev/mobile-dev, `deps check`. PRs now get an independent review before merge (AGENTS.md).
- L4 rubrics (phase 4): YAML cards for PRD and ADR, `internal/rubric` with a `Judge` interface and a scripted fake, `check.RubricFindings`; a planted weak PRD gets L4 findings, a good one none. `rubric.HarnessJudge` (any harness) and `nightshift check --standards --judge-auth` run them; the worker image still lacks `standards` (Dockerfile is owner-gated), so the runner does not call them yet.
- Polygon runs locally (ADR 0011): `nightshift eval run` makes a throwaway git repo per task in `evals/tasks/`, hidden tests grade it, no GitHub; one synthetic task so far. Memory page storage: `nightshift memory show|remember`, revision CAS, 40 entries / 4 KB; Lead's `memory[]` (owner quote or repo file as source) is built, chat commands open; page is in Lead's and Builder's prompts, daily prune runs.
- A stable instance builds the factory itself (runbook, "Stable instance"): `low` risk merges
  itself. Gaps: a return job starts from `main` without the last patch, a failed check's log or
  (after the first return) the review text; task text is cut at 120 characters.
- Deploy targets are designed (ADR 0014, design section 17): a `Deployer` interface with adapters `compose-local`, `script`, `external`; `compose-ssh` and `kubernetes` later. Phase 6 items P1–P6 are in the plan; Decision 13 is taken (the deployer may control Docker on the Mac). No phase 6 code yet.

- Promise checks (plan Q0): `cmd/nightshift/promises_test.go` fails on a prompt that names a missing command, a role skill that is not in `skills/`, a role key or schema field no code reads; every accepted gap has a plan item. Found: 21 schema keys are unread (role keys `inputs`, `fresh_context`, `on` were removed), among them the pool `monthly_usd`/`caps` (plan Q1, Q2); `project.protected` is read when a patch is applied and again on rebase (policy.ProjectGuard). Skill `self-audit` and AGENTS.md "Done means checked" are the rule for reports. Not verified: live runs; the weekly audit is only described in the PR.

## Next

Phase 5, Lead, is in progress. Full autonomy since 2026-11-25 (ADR 0013): merge on green after the independent review; ask the owner only about secrets, egress hosts, money, irreversible steps, weakened protections. Next, in order: (1) morning digest: `nightshift digest` and the 09:00 post (Slack) are built; `/ns status` shows the 24-hour summary (not the digest's merge list); (1b) done: a PR merged or closed outside the factory ends a task in any state with a PR; `nightshift cancel <task>`; (2) done: `/ns status|memory|remember` in chat; (3) done: `needs_research` back to Lead. Resume here: the prune rule for memory entries whose fact moved into the repo (design section 13), then hidden scenarios storage, tracker projection, `ns search`, #support, voice; M3 part 2 (per-tenant HOME for the harness) after the tenants thread's part 1. Fork CI runs and `refs/pull` stay out of verdicts and mirrors (#202). Defects from the live stand are fixed in code and tests, not yet re-checked live (the Mac thread does that): tool lockfiles restored after `mise install`, Builder gets the whole task text, a return round starts from the open PR's branch tip and reads the failing checks' summary and annotations (full CI logs need `actions: read` and the log host in the egress allowlist: the owner's call). Role overrides `.nightshift/roles/<name>.yaml` (instructions and skills only) are built.

- Audit follow-up (plan A1–A5) is done: one question list (`docs/decisions.md`); tests keep the design's compose example and subcommands equal to the code; `make drill` green on the Mac (22 of 22), which found and fixed the failing weekly restore test; rule for unverified outside facts in `AGENTS.md`; `claude -p` on a plan draws from the plan's limit (runbook "Who uses the model limit" has the query; decisions 10 and 12 are taken: the owner's instance uses his subscription, the credential is a per-instance option, Mac sessions merge their own PRs under a hook). Flaky: `TestCheckoutOfAMirrorOwnedByAnotherUser` failed once with ETXTBSY (a script written and run in parallel tests); audit it.
- Setup S1–S5 merged; the one-click app creation is not yet confirmed live. The factory's PRs are named by the approved spec; a task without a spec keeps its text. Research (2026-11-26): ADR 0018 says no to Temporal; `docs/research/` has the gap table; plan R1–R3 are what we take; `docs/prior-art.md` supersedes PR #233.
- `nightshift skills sync` is built (phase 4); `skills/vendor.yaml` holds four starter skills (decision 6); the worker skips the rest with a warning. Codex cost needs `price` on its provider (docs/setup.md); its CLI tries `chatgpt.com` and GitHub, blocked by egress, run unaffected.

- Several tenants on one instance and agent-made configuration (ADR 0015): `nightshift config validate`, the `configure-tenant` skill and `capacity.tasks` (plan C1, C2, M1), fair claims, one conductor loop per tenant (M2) and a GitHub token per tenant (M4) are built; M3 is half built (no checkout outlives its job; HOME per tenant is with the phase 5 thread); next M5.

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on an arm64 Mac: the pinned `hadolint` 2.12.0 (x86_64) segfaults; `gitleaks dir`
  also scans the ignored `.env`.

## Questions for the owner

All of them are in `docs/decisions.md`; this file keeps no list of its own.
