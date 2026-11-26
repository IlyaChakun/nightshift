# Parallel engineering and semantic conflicts

Prevent independently successful changes from producing an incompatible combined release. Extend the existing dependency graph, merge train and contract checks; do not introduce a second scheduler or serialize unrelated work without evidence.

**Priority:** P1 when concurrency grows. **Status:** research proposal, not approved implementation.

## Contents

- [Problem](#problem)
- [How it works](#how-it-works)
- [Trade-offs](#trade-offs)
- [Sources](#sources)
- [Fit](#fit)
- [Status](#status)
- [Proposal](#proposal)
- [Acceptance and stop conditions](#acceptance-and-stop-conditions)
- [Agent follow-up](#agent-follow-up)
- [Record and evidence](#record-and-evidence)
- [Decision brief — integrated candidate consistency, 2026-11-26](#decision-brief--integrated-candidate-consistency-2026-11-26)

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

## Decision brief — integrated candidate consistency, 2026-11-26

**Decision:** preserve parallel implementation and the current merge train, but prove that the exact integration candidate is evaluated against the actual target revision before irreversible actions. Do not build a new scheduler, distributed lock service or speculative merge queue before measuring contention.

### Pinned implementation and dispositions

| Evidence | Implication |
| --- | --- |
| [Task DAG](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/deps.go#L39-L91) rejects cycles, unknown links and duplicates; [queued task handling](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/conductor.go#L255-L305) applies dependencies and capacity. | Existing declared ordering and parallel capacity. A task DAG does not establish compatibility of independently planned changes. |
| [Ticker](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/tick.go#L108-L175) advances tasks sequentially within each tenant tick, with independent tenant loops; [ready/merging](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/gate.go#L341-L388) advances a ready task to merging and invokes the train. | Sequential calls are not necessarily exclusive train ownership across ticks while CI is pending. Inspect and prove one in-flight train candidate per repository/base, including restart and competing external merges. No full serialization audit performed. |
| [Train journal](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/book/migrations/00006_train_steps.sql) keys steps by task and step; [Train.Run/rebase](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/train/train.go#L123-L217) reuses a completed rebase SHA on later ticks and polls CI on that head. | Existing durable recovery; a completed rebase does not itself prove the target branch stayed unchanged while CI was running. Revalidate base and current candidate before merge. |
| [Git rebase](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/gitops/rebase.go#L33-L92) checks protected paths and compares stable patch IDs; [GitHub merge](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/adapters/github/github.go#L242-L255) supplies the expected PR head SHA to squash merge. | Head fencing is real. A matching patch ID is not a semantic compatibility proof. The merge request's head SHA check alone is not a compare-and-swap on target base. Effective host rules remain unverified. |
| [Train.Run](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/train/train.go#L144-L168) calculates ApprovalsKept, but its merge path uses CI outcome, and [conductor](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/gate.go#L374-L388) discards the returned result. | Reuse A01's explicit evidence/authority checks; do not implement a second approval system. |
| [Status](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/status.md#L17-L32) records two PRs adding migration 00015 and a subsequent repair. | Real reported example of a conflict not prevented by independent development; original run not reproduced. |

### Primary sources and alternatives

Checked 2026-11-26:

- [GitHub merge queue](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue): validates queue entries together with latest target and preceding changes. CI must support merge-group events. Official availability is organization-owned public repositories or qualifying organization private repositories; do not assume availability for this personal repository. Use its candidate construction principle, not a migration recommendation.
- [Git patch-id manual](https://git-scm.com/docs/git-patch-id): normalizes patch details to identify likely duplicate changes; stable mode also disregards file diff order and whitespace. It is not a behavioral equivalence check.
- [Pact can-i-deploy](https://docs.pact.io/pact_broker/can_i_deploy): versioned consumer/provider verification plus actual environment deployment records can answer whether a candidate is compatible with deployed peers. It covers declared/tested contracts, not all runtime interactions. Use a lightweight compatibility matrix first; no Pact Broker required.
- PR #344 records native dependency analysis and its coverage limitations. Treat footprint analysis as advisory; it cannot replace integrated tests.

| Option | Trade-off and disposition |
| --- | --- |
| Serial implementation per repository | Easy coordination, but reduces worker utilization and does not stop external base changes. Not default. |
| Parallel build, one fenced train candidate at a time | Simplest initial model. Requires exact revision binding, restart-safe ownership and final verification. Recommended. |
| Speculative queue with combined candidates | Can overlap CI for busy repos; invalidation and wasted builds when predecessors fail. Only when measured queue latency warrants it. |
| Contract-aware advisory scheduling | Can prevent recurrent conflicts before implementation; false-positive serialization and unknown dynamic edges. Pilot after final gates are sound. |
| Cross-repo reservations / release coordinator | Needed only for measured coordinated rollout problems; distinct releases are not atomic. Phase 10, not first pilot. |

### Reproducible semantic-conflict experiment

Executed 2026-11-26 in a disposable local Git repository, Git 2.47.3 and Python 3.13.5, no network or model calls. It tests the mechanism, not Nightshift's actual train.

Base contains policy.py with LIMIT=100, service.py importing LIMIT and returning amount <= LIMIT, and check.py asserting allowed(80) and not allowed(120). Branch A changes LIMIT to 90. Branch B changes the service comparison to amount + 15 <= LIMIT. Each branch independently passes the same check. Compute stable patch-id of B relative to base, then rebase B on A, compute stable patch-id relative to A, and run the identical check.

Observed results:

| Candidate | Check |
| --- | --- |
| A only | pass |
| B only | pass |
| B rebased on A | clean rebase; no unmerged paths; assertion allowed(80) fails |

Both B patch IDs were f4b99349f70963bdfc3b19f58eb8ba3a83f7c916. The same patch can change behavior when its surrounding system changes. This does not show that Nightshift misses this specific failure: if its fresh-candidate CI includes check.py it should detect it. It proves why neither textual merge success nor patch identity is sufficient evidence to reuse behavior-dependent approval.

### Candidate and ownership contract (proposal)

1. Preserve the existing DAG and independent worktrees. A planned footprint is a hint derived from task intent, known contracts and #344 impact brief; recompute actual footprint from the completed diff. Unknowns trigger broader checks, not relaxed policy.
2. Prove one active merge/release owner per tenant/repository/target (and, where needed, shared deployment environment) across pending CI, restart and multiple conductors. If missing, use the smallest Postgres-backed ownership/fencing mechanism in the existing train. Acquire transactionally; persist generation and release or reconcile after cancellation/crash. Do not hold a DB transaction across CI/network waits or assume expiry alone cancels an external merge.
3. Record the candidate identity: repository, target ref and observed base SHA, PR head, integration tree/commit, spec, policy, required checks, scenario suite and artifact digest. A clean rebase may carry forward a narrow patch-only review under explicit policy, but base-sensitive acceptance and authority must remain valid for the actual candidate. Recompute after external target movement, head movement, queue predecessor change or policy update.
4. Recheck the actual target immediately before merge. A read then unconditional write is not atomic. Use host enforcement of current-base required checks or an equivalent trusted fenced merge mechanism; inspect effective branch rules and bypass actors before claiming protection. If the host cannot guarantee that boundary, block unattended merge for the affected policy. Do not rely on expected head alone. Reconcile the actual merge commit/tree, verify required evidence still applies, and deploy only its authorized artifact; no fallback to an unverified head when actual merge identity is unknown.
5. For multiple repositories, model an explicit compatibility window: old producer/new consumer and new producer/old consumer where the rollout permits both, plus new/new and rollback combinations. Use additive schema/API expansion, migrate data/consumers, then contract after old versions disappear and rollback policy permits. Record actual deployed versions and compatibility results in a manifest. An incompatible transition requires a coordinated plan and human decision; never claim distributed atomic merge.

### Fault and compatibility matrix

| Scenario | Required result |
| --- | --- |
| Different files, same business invariant (fixture above) | Integrated candidate test rejects combination; no semantic evidence reused solely from patch-id |
| Target advances while CI pending, or external merge after final read | Rebuild/revalidate or host rejects; never authorize stale candidate |
| Two ready tasks, CI pending, second conductor/restart | One authorized train owner per target; unrelated builds still run |
| PR head moves or canceled attempt completes | Stale job/approval cannot authorize merge; current identity reconciled |
| Merge succeeds, reply lost, restart | Reconcile actual merge identity once; no duplicate merge/release |
| Duplicate migration version, incompatible schema, mixed versions | Deterministic migration/contract checks catch; rollout/rollback plan required |
| Two independent changes and one false overlap hint | Parallel build progresses; unnecessary serialization measured |
| Cross-repo old/new and new/old | Unsupported compatibility blocks affected rollout; unknown environment versions do not pass |

Run T4/T5 with the existing forge twin, real Git fixtures and Postgres for ownership races; then a disposable GitHub sandbox for target movement and host rules. Use independent acceptance against actual integrated behavior. Record conflict discovery stage, escaped conflicts, rework, queue age, accepted throughput, CI time, wasted speculative builds and false-positive scheduling hints. No claim of production safety from the toy experiment or a single green run.

**Go/no-go:** first fix confirmed A01 evidence gaps and demonstrate fresh-target fencing and combined checks. Add scheduling heuristics only if repeated semantic overlap materially costs time or acceptance; compare against unchanged parallel build + serial verified merge. Add speculation only when queue wait is measured as bottleneck. Keep cross-repo coordination deferred until a real tenant requires it and mixed-version compatibility/recovery are demonstrated.

**Checked:** pinned source paths, status, primary documentation and executed local Git/Python fixture. **Inferred:** candidate ownership protocol, host enforcement options, priorities. **Confidence:** high for the inspected path and fixture; medium for actual train race exposure. **Not verified:** complete locking inventory, effective host branch rules, Nightshift CI/local tests, live GitHub races or cross-repository rollout.
