# Phase 4 — Standards and the document quality loop

Owned by the session working on this phase; another session asks it and does not edit this file. Rules and limits: `docs/plan.md`.

Done when: see design section 22: a synthetic Rust project passes `tenant doctor` and `verify` without a pack; a planted weak PRD gets L1 and L4 findings; planted code defects are caught by L1 or L4; the polygon runs by `evals/selection.yaml`. Closed 2026-11-25; the leftovers below stay open.

## Status

- Last done: Done criterion met 2026-11-25 (packs, doc-lint, `skills sync`, polygon, the harness judge).
- Next: leftovers only (Vale and lychee, the standards book index, the 10-task polygon set, gates ADR 0014).
- Open: nothing blocks it.

## Items

- [ ] K1. `feat(check): doc-lint finds two ADRs with one number` — parallel tasks of a tenant pick the same next number under different names, and git merges that without a conflict; the finding fails the later PR once it is rebased on the base branch, as `scripts/repo-check.sh` does for this repository; T1.

## Scope and open work

Built: `go`, `python`, `ts` stack packs; their standards cards; doc-lint in `nightshift check` (ADR and PRD rules, `check.DefaultDocKinds`); discovery reads stacks from the packs' `detect` and names languages without a pack; a job with nothing to check says so (`check.nothing_to_run`). `standards list` and `standards sync` (base Go and Python configs from `standards/<set>/<stack>/files/`, never over a project's own file; the PR step comes with `tenant init`). Also built (ADR 0009, 0010): `mise install` and `project.yaml: setup` before the agent, run by the `toolchain` service with its own egress; Debian worker image; `tenant doctor` for a pinned toolchain; packs `js`, `java-maven`, `java-gradle`; own skills `research-options`, `build-vs-buy`, `web-dev`, `mobile-dev` (selected by `applies_to.deps`); `nightshift deps check`; `nightshift skills sync` (vendors `skills/vendor.yaml` by SHA, scans, writes a diff report; the starter list stays empty until the owner picks); L4 rubric cards (`standards/industry/docs/rubrics/`, `internal/assurance/rubric`, `check.RubricFindings` on a `Judge`; the live judge and the Verifier wiring are open). Open: project packs in `.nightshift/stacks/`, standards library, Vale and lychee, templates (PRD, ADR, design), the polygon and `nightshift eval`, the L2 research-record finding. Owner (2026-11-25): also stack packs `java-maven`, `java-gradle` and `js` (`rust` with the first Rust tenant), and skills `web-dev` and `mobile-dev` selected by dependencies (mobile means React Native, JS-level checks on Linux CI; native Android and iOS builds and skills, simulators and device e2e are "Later"). (Mutation tests are in design "Later" while an older plan line put them here: the owner picks.) Done: a synthetic Rust project passes `tenant doctor` and `verify` without a pack; a planted weak PRD gets L1 and L4 findings; planted code defects are caught by L1 or L4; the polygon runs by `evals/selection.yaml`. Polygon (owner 2026-11-25, ADR 0011): runs locally, a throwaway git repository per task, no GitHub; `eval run` built, 1 synthetic task. Closed 2026-11-25 (Done met); `skills sync` and the harness judge (`check --standards --judge-auth`) are in. Leftovers: Vale/lychee (`mise.lock` PR), the standards book index, the 10-task polygon set. Gates (ADR 0014, owner request 2026-11-25), open: `schemas/gates.json` and `project.yaml: gates`; the loader and `nightshift check --plan`; `block`, `warn` and `off` in the check loop with the floor; weakening is `critical` in the classifier; `/ns skip`; T1 and a T4 scenario with one `warn` check. MCP tools to read and, through a PR, change gates, blast radius and review mode (owner request, 2026-11-25).
