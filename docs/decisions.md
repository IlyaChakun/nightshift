# Pending decisions

The one list of questions that wait for the owner. Other documents link here and do not keep their own
lists. Limit: 80 lines.

Each entry has a question, the recommended option, whether it can be undone, and the date it was asked.
Rule (proposed, see the first entry): an entry marked **reversible** that is older than 24 hours is taken
with its recommended option by the session that finds it, recorded in the PR, and moved to "Taken". An
entry about secrets, egress, schemas, gates, the review policy, a new dependency or anything
irreversible is never taken that way.

## Waiting

| # | Question | Recommended | Undoable | Asked |
|---|---|---|---|---|
| 1 | Adopt the 24-hour rule above (needs a line in `AGENTS.md`) | Yes, reversible items only | yes | 2026-11-25 |
| 3 | Repository licence | Pick one before outside contributions | no | 2026-11-25 |
| 4 | The commit email in public history | Leave it, or rewrite history (breaks PR refs) | no | 2026-11-25 |
| 5 | Delete branch `test/ci-red` and merged branches | Delete | yes | 2026-11-24 |
| 7 | Remote MCP door: check live that the organization has the bearer-token beta and a connector reaches the tunnel (design section 25) | Check once on the Mac | yes | 2026-11-25 |
| 8 | Add `COPY standards` to the worker image so the runner can call rubrics (`Dockerfile` is owner-gated) | Add | yes | 2026-11-25 |
| 9 | Security leftovers after #202: CI verdict edge cases that fail closed or are unverified; details in the thread "Harden public repo security" | Handle in a follow-up PR | yes | 2026-11-25 |

## Taken

- 2026-11-25 (coordinator, reversible design cleanup), plan Q2: the role keys `inputs`, `fresh_context` and `on` are removed
  from `schemas/role.json` and the role files. No code read them: triggers are wired in the conductor and what a role
  sees is fixed in `jobsapi`. Design section 6 says so and narrows "a role is config" to access, skills, schema and limits.

- 2026-11-25 (taken by the session, 24-hour rule), decision 6: `skills/vendor.yaml` starts with the four skills the
  roles name that passed reading and a licence check (`obra/test-driven-development`,
  `obra/verification-before-completion`, `sentry/code-review`, `sentry/find-bugs`). Left out: `playwright/playwright-cli`
  (runtime `npm install`) and the Go, Python and security sets, which design section 12 selects on the polygon.

- 2026-11-25 (taken by the session, 24-hour rule), decision 11: PR #85 (dev-process batch) was already merged by the
  owner on 2026-11-25 08:38 UTC, so nothing is left to approve.
- 2026-11-25 (owner, 19:34 UTC), decision 13: the `deployer` job may control the Docker engine on the owner's
  Mac for `compose-local`, behind the compose-file guard (no privileged mode, host mounts or Docker socket;
  loopback-only ports; CPU and memory limits), with a `preview` network and exact preview hosts in egress
  (ADR 0014). The code waits for phase 6 (plan P4).

- 2026-11-25: the install host list in `deploy/egress-install-acl.yaml` is confirmed, with
  `downloads.sqlc.dev`, `registry.npmjs.org` and `tuf-repo-cdn.sigstore.dev`; Java and Rust
  hosts are asked per instance.
- 2026-11-25: of this repository the factory may merge alone only plain documents under
  `docs/` (`low`, up to 80 lines); the rest waits for a person
  (`.nightshift/blast-radius.yaml`). Reason: it is one binary that holds every secret, and
  three review passes kept finding files outside the obvious gate folders that switch off a
  check or instruct an agent. Taken by the session (strengthening, reversible): widen it when
  the factory has its own independent review and a narrower split is proven.
- 2026-11-25 (owner, 19:15 UTC): a Mac session merges its own PRs itself, under a narrow allow rule
  `Bash(gh pr merge *)` and a `PreToolUse` hook that refuses unless checks are green, no protected path
  is touched and the review label is set; a cloud thread never merges a Mac session's PR. The Mac thread
  implements the rule and the hook (see docs/runbook.md, "Merge from a Claude Code session").
- 2026-11-25 (owner, 19:20 UTC): the owner's own instance keeps running the factory on his
  subscription (`claude-sub`); it is his factory. The model credential stays a per-instance option
  (`claude-sub` token, `claude-api` key, `codex-api` key; docs/setup.md, step 2), so other operators choose
  for themselves. Builder keeps the wait-then-fallback chain of ADR 0012; no separate API pool is added.
