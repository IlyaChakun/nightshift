# Persist required Builder and Verifier decisions

State: open

Source: [audit #334](https://github.com/IlyaChakun/nightshift/pull/334). Builder returns and Verifier risk increases commit with job consumption. The Postgres regression tests reject each event and verify that the same result remains available. See [ADR 0023](../../adr/0023-durable-dispatch-decisions.md).

The [live check](../../live-checks/A03a-durable-dispatch-decisions.md) is pending.
