# Store one validated copy of each Verifier finding

State: open

Source: [audit #337](https://github.com/IlyaChakun/nightshift/pull/337). Jobs API writes the finding at completion and the conductor reuses it. See [ADR 0024](../../adr/0024-single-verifier-finding-writer.md).

The PostgreSQL and Jobs API scenario checks that the rule, layers, location, message and fingerprint survive dispatch with one row. The [live check](../../live-checks/A06a-single-verifier-finding-writer.md) is pending.
