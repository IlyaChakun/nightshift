# Plan

Phases and their "Done" criteria are defined in `docs/design.md` (section 22).
This file tracks the current phase only. Limit: 150 lines.
Rules: one item = one PR, under ~400 changed lines. Tick an item in the PR that completes it.
Add an item only if it follows from the design and the owner agreed.

Phase 0 (skeleton and contracts) is done: PRs #1–#13.

## Current phase: 2 — runner, harnesses and isolation

Phase 1 is done in code (PRs #14–#29). Its Slack check (item 9) moved to the owner's local test:
`docker compose up`, the bot answers in a thread, a restart keeps state, a restore works.

- [x] 1. `feat(book): add jobs, job tokens, harness sessions and artifacts` — lease, attempts,
  token hashes only; T3
- [x] 2. `feat(jobsapi): serve claim, heartbeat, events and complete` — worker token checked
  against its sha256, job tokens scoped to task and role and expiring with the lease; T1, T3
- [x] 3. `feat(gitops): add repository mirrors and task worktrees` — bare mirror in `repos`,
  one worktree per task, patch applied and pushed only from `nightshift`; T1 on local git
- [ ] 4. `feat(runner): add nightshift work` — claim loop, job directory, harness run as the agent
  user, schema check, patch and artifact sha256, complete; e2e on `fake`: job → worktree → result
  → artifacts
- [ ] 5. `feat(harness): add the claude adapter` — `claude -p` with JSON schema, explicit
  permissions and settings, `--bare` for API providers, subscription env scrubbed; T1 on a stub
  binary, T8 opt-in
- [ ] 6. `feat(dispatch): add provider router and task budget` — providers `claude-sub`
  (`concurrency: 1`), `claude-api`, `claude-cheap`, switch on limit, park on exhaustion,
  harness sessions per (task, role); T1, T3
- [ ] 7. `feat(deploy): add the worker image and isolated networks` — users runner and agent,
  agent HOME volume, networks `core`, `jobs`, `ci`, `ui`, Smokescreen `egress` and `egress-ci`;
  T6: no GitHub token, no host outside the allowlist, no local network, no Postgres
- [ ] 8. `test(jobsapi): a Builder token never sees hidden scenarios` — T6
- [ ] 9. `feat(deploy): add the obs profile` — `lgtm`, Claude Code telemetry, `TRACEPARENT` from
  the runner so agent spans land in the task trace
- [ ] 10. Done check: a real `claude` job shows in Grafana as one trace (needs an API key)

Owner requests outside the phase list: [x] Go style standard (`standards/industry/go/`) and stricter
`.golangci.yml`; [x] secrets hardening before item 9: `book_backup` role instead of the superuser,
no `os.Getenv` in Go code, URL passwords masked, rotation steps in the runbook. [x] Engineering disciplines map in design section 9 (docs only; work lands in phases 4 and 6).

## Phase 2 done when

- An e2e test on `fake` passes "job → worktree → result → artifacts".
- A real `claude` job shows in Grafana as one trace.
- The agent finds no GitHub token and cannot reach a host outside the allowlist, the local
  network or Postgres; a Builder token does not get hidden scenarios; T6 tests check all of this.

## Next phases

3 — Builder, checks, gates.
Phases 4–10 start only when the previous ones hurt without them (design section 22).
