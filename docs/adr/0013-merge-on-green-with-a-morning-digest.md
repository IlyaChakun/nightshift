---
status: accepted
date: 2026-11-25
---

# Sessions merge on green, including gated paths; the owner gets a morning digest

## Context and Problem Statement

Until now a PR that touched a gated path (`.github/`, `deploy/`, `schemas/`, dependencies, `AGENTS.md` and the like) waited for the owner, so the owner approved many PRs a day, most of them routine. The owner wants the factory to merge nearly everything itself, working alone at night and asking by day only about very important things, while keeping control. How much should the factory decide alone, and how does the owner stay in control?

## Considered Options

- Keep the gated-path list and batch the approvals
- Full autonomy: merge everything that passes the gates; ask only about a short list of owner-only topics; control through a digest and cheap reverts
- Autonomy by time: merge gated paths only at night

## Decision Outcome

Chosen option: "Full autonomy with a short owner-only list and a digest", because the gates (CI, CodeQL, the independent review by an agent that did not write the change, the checks in `nightshift check`) are what protects `main`, and a label does not add protection that the review does not already give.

- **Merge on green.** A session merges its own PR when `make verify`, CI and CodeQL are green and the independent review found no unfixed bug, including PRs under gated paths and new dependencies (the dependency research record and the L2 rule still apply).
- **The owner is asked only about** secrets and accounts, a new egress host, money, an irreversible step, and weakening a protection of the factory (a gate, the review policy, branch protection, isolation). Strengthening needs no question.
- **Control is a digest, not a queue.** The morning digest (the 09:00 task and `/ns status`) lists every PR merged and every decision taken since the last one, each with its reason and a revert link; `needs-owner-review` marks the gated ones so they can be read first. Reverting is one click on a revert PR, which the same rules let a session merge.

### Consequences

- Good, because the owner stops being the bottleneck; the factory runs day and night.
- Bad, because a bad gated change can reach `main` before the owner sees it. Mitigation: the independent review is mandatory, the digest names the gated PRs, and a revert is cheap.
- Bad, because "weakens a protection" is a judgement. Mitigation: when in doubt the session asks.
