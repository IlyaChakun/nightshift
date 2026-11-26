# Confirm handoff findings with comparable checks

State: open

Source: [audit #337](https://github.com/IlyaChakun/nightshift/pull/337). Follow-up to [finding handoff](A06b-finding-handoff-status.md).

A local finding is resolved by a completed run of the same suite. A CI finding is resolved after the original failing checks each report success. Tests cover suite changes, incomplete reports, unrelated green CI, and skips. The [live check](../../live-checks/A06b-check-evidence.md) is pending.
