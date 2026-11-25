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
| 10 | Builder on the owner's subscription or on its own pool (audit, finding 5; ADR 0012) | Measure first (plan A5), then propose | yes | 2026-11-25 |
| 11 | Approve the dev-process batch (PR #85); check first whether it is still open | Approve | yes | 2026-11-24 |
| 12 | Which factory pull requests may merge with no person. Today the rules hold everything but plain documents; letting `medium` code merge alone also lets it change what runs with the factory's secrets, because all of it is one binary | Keep only documents automatic until the factory has its own independent review | yes | 2026-11-25 |

## Taken

- 2026-11-25: the install host list in `deploy/egress-install-acl.yaml` is confirmed, with
  `downloads.sqlc.dev`, `registry.npmjs.org` and `tuf-repo-cdn.sigstore.dev`; Java and Rust
  hosts are asked per instance.
