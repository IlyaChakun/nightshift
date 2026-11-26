# Verify finding status across a Builder handoff

Status: pending

Source: [A06b](../plan/phase-5/A06b-finding-handoff-status.md) and [audit #337](https://github.com/IlyaChakun/nightshift/pull/337).

On an isolated staging tenant, have Verifier return one blocking finding. Check that enqueueing Builder shows `handed_off` in `book.finding_states`. Run a follow-up review that still reports it, then one that passes after the fix. Expect the finding to become `fixed` after the passing review acknowledges its recheck. Keep the job IDs, verdicts and finding rows as evidence.
