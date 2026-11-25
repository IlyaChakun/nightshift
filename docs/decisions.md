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
| 6 | Starter skill list for `skills/vendor.yaml` (design section 25) | Go, Python, review, security, Playwright | yes | 2026-11-24 |
| 7 | Remote MCP door: check live that the organization has the bearer-token beta and a connector reaches the tunnel (design section 25) | Check once on the Mac | yes | 2026-11-25 |
| 8 | Add `COPY standards` to the worker image so the runner can call rubrics (`Dockerfile` is owner-gated) | Add | yes | 2026-11-25 |
| 9 | Security follow-ups from the review of #182: over 5000 reviews stall a task; the mirror fetches `refs/pull/*`; a fork's run on the same SHA may supersede CI in the Actions fallback | Fix the first two, check the third | yes | 2026-11-25 |
| 11 | Approve the dev-process batch (PR #85); check first whether it is still open | Approve | yes | 2026-11-24 |
| 13 | Phase 6 `compose-local`: let a `deployer` job reach the host's Docker engine (root-equivalent there), behind a recipe lint, and add a `preview` network with exact allowed hosts to egress (ADR 0014; widens isolation, so never taken by default) | Yes, only on your own instance; loopback ports, CPU and memory limits, no host mounts | yes | 2026-11-25 |

## Taken

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
