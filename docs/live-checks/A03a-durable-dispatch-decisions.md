# Required events during dispatch recovery

- What. [A03a](../plan/phase-3/A03a-durable-dispatch-decisions.md), from [audit #334](https://github.com/IlyaChakun/nightshift/pull/334).
- Run. On an isolated staging stack with a disposable database, refuse the write for a red Builder result and then for a Verifier pass that raises risk. Remove each fault and observe the next conductor ticks.
- Expect. The attempt remains unconsumed while the write fails. After recovery, the Builder return is counted once and the Verifier risk increase requires human review.
Status: pending
