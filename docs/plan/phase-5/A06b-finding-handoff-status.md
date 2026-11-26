# Track when a finding is handed to Builder and verified fixed

State: open

Source: [audit #337](https://github.com/IlyaChakun/nightshift/pull/337). Keep findings distinguishable when Builder has received them and when a later check proves the fix. Check current status consumers and old records before changing the status contract.

Postgres tests cover failed handoff, distinct findings without fingerprints, and a repeat report followed by a passing Verifier review with explicit recheck targets. The [live check](../../live-checks/A06b-finding-handoff-status.md) is pending.
