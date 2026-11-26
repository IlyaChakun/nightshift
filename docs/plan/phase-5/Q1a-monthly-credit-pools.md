# Reserve shared monthly credit before a model run

State: open

Source: Q1, design section 12, [ADR 0012](../../adr/0012-limits-wait-then-fallback.md). The owner chose a strict shared monthly ceiling on 2026-11-26. Reserve the maximum charge before starting. Hold a task when its price or maximum charge is unknown.

The jobs API checks eligibility before leasing. Current CLI harnesses report usage after a model message, which cannot guarantee its maximum bill. A numeric ceiling therefore holds their jobs. Uncapped providers remain available.

Open: support a paid harness with a verifiable pre-call charge bound, atomically reserve the bound across all tenants of the pool, and reconcile on completion. Retain unresolved charges across worker loss, retries, restarts and month boundaries. Test concurrent claims for the last credit and prove a retry cannot spend twice on one reservation. Per-work-category caps remain part of Q1.
