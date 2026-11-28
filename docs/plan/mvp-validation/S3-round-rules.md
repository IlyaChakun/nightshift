# Validation round rules: fewer rounds, fewer waits

State: proposed

Source: `docs/research/mvp-validation-timeline.md`. Proposed edits to "Rules for this stream" and "Freeze rule" in this folder's `README.md`, for the session that owns it:

1. **Run every stage that does not depend on a failed one in the same round.** Record each result; stop at the end of the round, not at the first failure. On 2026-11-28 round 1 stopped after V1 and V2, and V3's 409 showed up two rounds later. "A stage that fails the same way twice stops the run" stays.
2. **Ask the owner everything a run needs in V0, in one batch**: tenant providers (Verifier), egress choices, bot invites, sandbox recipes, the GitHub clicks. On 2026-11-28 these questions held runs for 57 min.
3. **No docs pull request merges into `main` during the freeze** unless it records validation results or the owner made an exception in words. On 2026-11-28 seven README PRs moved `main` and left a green fix stale twice.
4. **After each fix merges, the coordinator wakes the Mac thread at once** with the commit to upgrade to. A message restarted the offline session in 14 s on 2026-11-28.

Done when: the README carries these rules and the next validation round follows them.
