# Recover an interrupted Verifier verdict

- What. [A03b](../plan/phase-3/A03b-atomic-verdict.md), from [audit #334](https://github.com/IlyaChakun/nightshift/pull/334).
- Run. On an isolated staging stack with a disposable book, refuse the review transition for a completed Verifier return with findings and raised risk. Remove the fault and restart the conductor.
- Expect. The failed transaction leaves the result available and the task in review. The restart applies the same result once, records its risk and findings, and returns the task to Builder.
- Status: pending.
