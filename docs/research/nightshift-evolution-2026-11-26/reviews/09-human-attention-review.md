# Human attention and pending decisions review (PR #348)

Give the owner a complete list of pending decisions before experimenting with ranking. An inbox card, a digest entry and a recorded decision represent different events in the current implementation. The proposal offers a credible way to measure operator burden, but it has no Nightshift measurements demonstrating that a new console or notification policy would save time.

Reviewed source `796f2bc` against `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition | Evidence |
| --- | --- | --- |
| CLI and MCP share decision handling, with cards stored by the inbox adapter | Confirmed. The adapter records deliveries and the inbox scans recent task events. Full parity across live doors is unverified. | `internal/dispatch/doors.go:73`, `internal/adapters/chat/inbox/inbox.go:36`, `docs/plan/doors.md:19` |
| The digest omits some pending work | Confirmed limitation. It uses selected states, suppresses older matching titles and shows five entries plus a remainder. Dependency blockers can request owner input. | `internal/summary/summary.go:22`, `internal/summary/summary.go:55`, `internal/summary/summary.go:197`, `internal/dispatch/deps.go:233` |
| An action needs the revision the operator actually reviewed | Partly. Versioned cards and transactional state transitions exist. The production consequence of the source's stale-intent hypothesis is unverified. | `internal/dispatch/doors.go:48`, `internal/flow/move.go:25`, design §15 |
| Spec cards use the blocker kind and S2 still awaits live checking | Partly outdated. Spec cards use `CardSpec`; quiet hours hold two other card kinds. The S2 ledger records live CLI/Grafana evidence. Lead already includes project memory and similar tasks. | `internal/dispatch/lead.go:364`, `internal/dispatch/quiet.go:27`, `docs/plan/phase-5.md:30`, `internal/dispatch/lead.go:230` |

## Suggested decisions

- **Take, phase 5 and doors I7.** Derive the full tenant-scoped list from outstanding decisions, with stable identity, purpose, applicable revision, options, evidence and expiry. Preserve mandatory items across replies, digest truncation, delivery failures and duplicate submissions. Require equivalent actor, identity and revision checks at commit across authorized doors; retain the remote door's existing scope. Add storage only if the book cannot represent this.
- **Take, phase 5 pilot.** Compare the current digest with a complete pending list on matched scenarios. Count active minutes voluntarily supplied, unanswered items, corrections, repeated questions, wait times and accepted outcomes across all starts. Keep elapsed wait separate and record missing minutes. Avoid retaining private message text for timing.
- **Later, phase 7.** Trial templates, ranking, coalescing and quiet-hour batching only after completeness is demonstrated. Measure interruptions and decision quality together. Revisit a dedicated web console only for observed friction.
- **Reject, phase 5.** Additional automatic approvals or a summary that can dismiss mandatory decisions would expand authority. Respect the existing explicitly configured review SLA. See `internal/dispatch/reviewwait.go:26`.

The productivity study observed “significant task and activity switching while still feeling productive.” It supports asking the operator about interruptions without assigning a universal cost per message. Read 2026-11-26. [Meyer et al.](https://www.microsoft.com/en-us/research/publication/software-developers-perceptions-of-productivity/)

**Checked** a temporary Go test of two distinct pending tasks with the same title returned “listed 1 of 2 distinct pending requests.” Also checked doors, inbox, digest, quiet hours, SLA, memory and phase ledger. **Inferred** projection design. **Unverified** live cross-door parity, stale-action outcomes and attention savings. **Confidence** high for current display behavior, medium for pilot benefit.
