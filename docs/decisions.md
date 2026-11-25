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
| 10 | Builder on the owner's subscription (shares the plan limit with the owner's own sessions, docs/runbook.md "Who uses the model limit") or on an API provider with a monthly cap (money) or a cheaper model through `codex-api` | Run the runbook query on the Mac first; if Builder is most of the `claude-sub` use, give Builder an API provider and keep the subscription for Lead and your own sessions; otherwise stay and set `weekly_share` | yes | 2026-11-25 |
| 11 | Approve the dev-process batch (PR #85); check first whether it is still open | Approve | yes | 2026-11-24 |
| 12 | How a Mac session merges its own PRs without you pressing the button and without bypassing Claude Code's protection: (a) narrow allow rule `Bash(gh pr merge *)` in your user settings plus a `PreToolUse` hook that refuses unless checks are green, no protected path is touched and the review label is set; (b) you press merge; (c) the factory merges them as tasks | (a) after the Mac thread runs the five diagnostics in docs/runbook.md and names the real cause; needs your OK, because `CLAUDE.md` forbids adding hooks or changing settings without it | yes | 2026-11-25 |

## Taken

- 2026-11-25: the install host list in `deploy/egress-install-acl.yaml` is confirmed, with
  `downloads.sqlc.dev`, `registry.npmjs.org` and `tuf-repo-cdn.sigstore.dev`; Java and Rust
  hosts are asked per instance.
