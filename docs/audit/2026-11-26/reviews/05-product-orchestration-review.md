# A05 review — acceptance of a decomposed objective

Pilot one integrated acceptance check for a two-task feature. Nightshift already validates task dependencies and releases individual tasks. The audit identifies a product-level question, but shows no instance of an objective being reported complete incorrectly.

Checked revision: `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition | Evidence |
|---|---|---|
| Named criteria and a validated task graph exist | Confirmed | `schemas/spec.json:14` defines criterion IDs, Given/When/Then and source quotes. Task entries at line 39 define dependencies. `internal/dispatch/deps.go:41` checks duplicate IDs, unknown dependencies, self-edges and cycles. |
| Expansion links children through dependencies and a thread | Confirmed | `internal/dispatch/deps.go:128` locks and rechecks expansion, reuses the first task, creates siblings with a shared thread and records dependency edges. Its local ID mapping is not written as a membership record. |
| Child review is task-scoped | Confirmed | `internal/dispatch/verify.go:161` omits the whole spec after expansion. Builder uses task-scoped context at `internal/dispatch/build.go:230`. Hidden scenarios are indexed by tenant and task at `internal/dispatch/hidden.go:51`. |
| A child release proves the entire feature works | Unverified | Dependency readiness reads child state at `internal/dispatch/deps.go:198`. The train finishes a specific task at `internal/train/train.go:247`. No objective-completion event was identified in those paths. |

**Take — phases 5 and 6.** Use the existing Lead, Builder, Verifier and DAG for a two-change fixture with one criterion spanning both changes. Record child review inputs, explicit membership including an independent sibling, spec revision, criterion IDs, integrated commit and independently observed behavior. Include incompatible interfaces and a corrected candidate. Record blocked, canceled and incomplete work separately. The pilot succeeds if it can distinguish both outcomes without inferring completion from child count.

**Later — phase 5.** If this pilot shows a need for product-level completion, preserve the approved spec revision, membership and criterion coverage during expansion. Reuse existing checks to assess the combined tree. Require renewed evidence after a relevant revision or contract change. Bound repair and require authorization for revised requirements. Keep acceptance, release and post-release user outcome separate. Stable requirement identifiers serve this limited purpose. NASA recommends identifying each requirement “by unique identifier”. Read 2026-11-26. [NASA verification matrix](https://www.nasa.gov/reference/appendix-d-requirements-verification-matrix/)

**Later — phase 10.** Evaluate manifests, compatible deployment order and optional feature flags when the first multi-repository objective requires them. **Reject — phases 5–6.** A second graph scheduler, dynamic planning engine, new permanent role or dedicated semantic-conflict analyzer. Existing deterministic graph validation and an integrated contract test cover the pilot.

**Checked** — local `TestCheckTaskGraph` passed. Read spec schema, expansion SQL, role inputs, hidden storage, train, design §§6–8 and phase 10. **Inferred** — value of explicit objective membership. **Unverified** — effective live prompts, full multi-task delivery, objective misreporting and selective invalidation. **Confidence** — high for per-task behavior, medium for proposed scope.
