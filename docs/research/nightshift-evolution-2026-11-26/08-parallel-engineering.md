# Parallel engineering and semantic conflicts

Prevent independently successful changes from producing an incompatible combined release. Extend the existing dependency graph, merge train and contract checks; do not introduce a second scheduler or serialize unrelated work without evidence.

**Priority:** P1 when concurrency grows. **Status:** research proposal, not approved implementation.

## Problem

Textual merge success does not imply semantic compatibility. Two agents may change different files but break a shared API, database invariant, authentication rule or operational assumption. Parallel migrations can claim the same version or apply incompatible schema changes. Passing tests at individual branch heads does not prove that the combined candidate works.

## How it works

Represent the planned change footprint as affected components, contracts, data schemas and declared dependencies. Treat it as a scheduling hint, not a security authority. Identify overlapping high-risk footprints before dispatch, and recompute actual footprints from the completed diffs. Unexpected overlap triggers a targeted check or coordination step.

The existing merge train constructs and verifies the exact integration candidate against a fresh base. Rebind acceptance and approval evidence when material inputs change. For coordinated multi-repository features, maintain a versioned release manifest, explicit compatibility windows and an ordered rollout/rollback plan; never pretend separate GitHub merges are one atomic transaction.

## Trade-offs

| Option | Benefit | Limitation |
| --- | --- | --- |
| Serialize all tasks per repository | Simple consistency model | Wastes parallel capacity |
| Existing merge train plus combined-candidate tests | Stronger evidence with little scheduling change | Conflicts may be found late |
| Advisory footprint and contract-aware scheduling | Prevents expensive rework | Analysis is incomplete |
| Distributed reservation or locking system | Coordinates declared shared resources | Complexity, stale locks and false contention |

Start with the merge train and deterministic contract checks. Add advisory reservations only for repeated, measured conflict classes; use scoped leases and recovery if reservations become necessary.

## Sources

- [GitHub merge queue documentation](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue), search excerpt checked 2026-11-26: combined merge candidates and required checks provide a useful reference, not a recommendation to replace the train.
- [Bazel query how-to](https://bazel.googlesource.com/bazel/+/staging/site/docs/query-how-to.md), search excerpt checked 2026-11-26: reverse dependencies are a research lead for impact detection.
- Existing Nightshift research on durability and prior art in `docs/research/durable-execution.md` and `docs/prior-art.md`.

## Fit

Design section 7 specifies task dependencies, merge train, fresh-base checks and rollback. Section 5 describes API contract tools. Phase 10 addresses multi-repository work. PR #332 covers evidence provenance; PR #336 covers product-level completion. Build on these rather than inventing new state machines.

## Status

Partial foundation: task dependencies and a journaled train are documented in plan; a migration collision is recorded in status as a real example. Completeness of semantic conflict detection and final-candidate acceptance is unverified.

## Proposal

Create three deterministic integration scenarios: parallel changes to API producer and consumer, incompatible migrations, and independent edits to a shared authorization invariant. Each pair passes in isolation but must fail or coordinate when combined. Add a non-conflicting pair to test unnecessary serialization.

Track base and head revisions, integration-candidate hash, contract/test versions, deployment artifact and review decision. If the candidate changes, rerun affected checks and require renewed approval where policy requires it. For unknown dependency impact, retain all mandatory checks. Compare conflict escape rate, rework time, queue latency and parallel throughput before introducing scheduling heuristics.

## Acceptance and stop conditions

- Each planted semantic conflict is detected before release or explicitly blocked for missing evidence.
- Required gates run on the actual combined candidate; stale evidence cannot authorize a changed artifact.
- Two unrelated tasks still make progress concurrently.
- Retry, cancellation and worker death cannot leave permanent reservations.
- Multi-repository rollout is deferred until version compatibility and recovery are demonstrable.
- Stop adding scheduling complexity if combined-candidate checks eliminate the measured failures at acceptable cost.

## Agent follow-up

Read PRs #332, #334, #336 and #344; verify the existing train and deployment code before proposing changes. Use T4/T5 scenarios and independent acceptance rather than relying on branch-local green CI.

## Record and evidence

Preserve this dated owner-requested proposal. Link later experiments, decisions and implementation PRs; do not silently delete or replace the source record. The living design remains `docs/design.md` and the canonical sources index is `docs/prior-art.md`.

**Checked:** [base 9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; design sections 5, 7, 8, 11, 17, 20, 22, plan, status, research index and open PR titles. **Inferred:** candidate gaps, priorities and experiment design. **Confidence:** medium; no live validation or full code audit. External links identified above were checked only to the extent stated.
